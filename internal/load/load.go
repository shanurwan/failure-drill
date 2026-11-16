// Package load sends requests at a fixed rate and keeps what happened to each one.
//
// The rate is open-loop: a request is started on every tick whether or not
// earlier ones have finished. A gateway that stalls therefore shows up as
// timeouts, the way it would for real callers, instead of quietly slowing the
// test down.
package load

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Target is one URL to call, with the headers its API needs.
type Target struct {
	Name   string
	URL    string
	Header http.Header
}

// Sample is the outcome of one request.
type Sample struct {
	// T is when the request was sent, in seconds since the run started.
	T float64 `json:"t"`
	// Status is the HTTP status, or 0 when no response arrived.
	Status int `json:"status"`
	// Ms is how long the caller waited.
	Ms float64 `json:"ms"`
	// Err classifies a request that got no response: timeout, refused, reset,
	// eof or other.
	Err string `json:"err,omitempty"`
}

// OK reports whether the caller got a usable answer.
func (s Sample) OK() bool { return s.Status >= 200 && s.Status < 400 }

// Options control a run.
type Options struct {
	RPS     int
	Timeout time.Duration
}

// Run calls every target at opts.RPS until ctx ends, then waits for requests in
// flight and returns the samples per target, ordered by send time.
func Run(ctx context.Context, start time.Time, targets []Target, opts Options) map[string][]Sample {
	out := make(map[string][]Sample, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, target := range targets {
		client := &http.Client{
			Timeout: opts.Timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     30 * time.Second,
				DialContext:         (&net.Dialer{Timeout: opts.Timeout}).DialContext,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
		wg.Add(1)
		go func(target Target) {
			defer wg.Done()
			var inFlight sync.WaitGroup
			ticker := time.NewTicker(time.Second / time.Duration(opts.RPS))
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					inFlight.Wait()
					return
				case <-ticker.C:
					inFlight.Add(1)
					go func() {
						defer inFlight.Done()
						sample := call(client, target, start)
						mu.Lock()
						out[target.Name] = append(out[target.Name], sample)
						mu.Unlock()
					}()
				}
			}
		}(target)
	}
	wg.Wait()

	for name := range out {
		sortByTime(out[name])
	}
	return out
}

func call(client *http.Client, target Target, start time.Time) Sample {
	sent := time.Now()
	sample := Sample{T: sent.Sub(start).Seconds()}

	// The request deliberately outlives the run's context: a request sent just
	// before the end is still a request a caller made.
	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		sample.Err = "other"
		return sample
	}
	for key, values := range target.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := client.Do(req)
	sample.Ms = float64(time.Since(sent).Microseconds()) / 1000
	if err != nil {
		sample.Err = classify(err)
		return sample
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	sample.Status = resp.StatusCode
	return sample
}

func classify(err error) string {
	var netErr net.Error
	switch {
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		return "reset"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), strings.Contains(err.Error(), "EOF"):
		return "eof"
	default:
		return "other"
	}
}

func sortByTime(samples []Sample) {
	// Samples arrive nearly sorted; insertion sort keeps this dependency-free
	// and is fast on that input.
	for i := 1; i < len(samples); i++ {
		for j := i; j > 0 && samples[j].T < samples[j-1].T; j-- {
			samples[j], samples[j-1] = samples[j-1], samples[j]
		}
	}
}
