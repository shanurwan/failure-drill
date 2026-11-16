// Package drill runs one failure against a live install while traffic flows,
// and records what callers saw.
package drill

import (
	"context"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/shanurwan/failure-drill/internal/kube"
	"github.com/shanurwan/failure-drill/internal/load"
	"github.com/shanurwan/failure-drill/internal/report"
)

// Options are common to every drill.
type Options struct {
	Targets  []load.Target
	RPS      int
	Timeout  time.Duration
	Baseline time.Duration
	Observe  time.Duration
	Log      io.Writer
}

// Event is something the drill did or saw, in seconds since the fault.
type Event struct {
	T    float64 `json:"t"`
	Kind string  `json:"kind"`
	Text string  `json:"text"`
}

// TargetResult is what one target's callers saw.
type TargetResult struct {
	Name    string         `json:"name"`
	URL     string         `json:"url"`
	Summary report.Summary `json:"summary"`
	Samples []load.Sample  `json:"samples"`
}

// Result is the full record of a drill.
type Result struct {
	Drill     string    `json:"drill"`
	Fault     string    `json:"fault"`
	Namespace string    `json:"namespace"`
	Started   time.Time `json:"started"`
	RPS       int       `json:"rps"`
	TimeoutMs int64     `json:"timeout_ms"`
	BaselineS float64   `json:"baseline_s"`
	ObserveS  float64   `json:"observe_s"`
	// InjectAt is when the fault was injected, in seconds since Started.
	InjectAt float64 `json:"inject_at_s"`
	// Facts are what the drill learned about the system itself: which pod was
	// primary, how long a promotion took, and so on.
	Facts   map[string]string `json:"facts"`
	Events  []Event           `json:"events"`
	Targets []TargetResult    `json:"targets"`
	// Output is what the command printed, kept so a record can be replayed.
	Output string `json:"output"`
}

// A Fault is the part that differs between drills.
type Fault interface {
	// Name is the drill's name and Kind the way the fault is injected.
	Name() string
	Kind() string
	// Prepare checks the system is in a state worth breaking and returns a
	// one-line description of what is about to happen.
	Prepare(ctx context.Context, k *kube.Client) (string, error)
	// Inject breaks it.
	Inject(ctx context.Context, k *kube.Client) error
	// Watch is called about once a second from before the fault until the
	// drill ends. It reports changes through note and facts through fact.
	Watch(ctx context.Context, k *kube.Client, note func(kind, text string), fact func(key, value string))
}

// Run performs the drill: baseline traffic, the fault, then observation.
func Run(ctx context.Context, k *kube.Client, fault Fault, opts Options) (*Result, error) {
	plan, err := fault.Prepare(ctx, k)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	result := &Result{
		Drill:     fault.Name(),
		Fault:     fault.Kind(),
		Namespace: k.Namespace,
		Started:   start.UTC(),
		RPS:       opts.RPS,
		TimeoutMs: opts.Timeout.Milliseconds(),
		BaselineS: opts.Baseline.Seconds(),
		ObserveS:  opts.Observe.Seconds(),
		InjectAt:  opts.Baseline.Seconds(),
		Facts:     map[string]string{},
	}

	var mu, faultMu sync.Mutex
	injected := start.Add(opts.Baseline)
	note := func(kind, text string) {
		mu.Lock()
		t := time.Since(injected).Seconds()
		result.Events = append(result.Events, Event{T: tenths(t), Kind: kind, Text: text})
		mu.Unlock()
		fmt.Fprintf(opts.Log, "  %+7.1fs  %s\n", t, text)
	}
	fact := func(key, value string) {
		mu.Lock()
		result.Facts[key] = value
		mu.Unlock()
	}

	fmt.Fprintf(opts.Log, "drill      %s (%s)\n", fault.Name(), fault.Kind())
	fmt.Fprintf(opts.Log, "namespace  %s\n", k.Namespace)
	for _, target := range opts.Targets {
		fmt.Fprintf(opts.Log, "traffic    %-6s %d req/s  %s\n", target.Name, opts.RPS, target.URL)
	}
	fmt.Fprintf(opts.Log, "plan       %s\n\n", plan)

	loadCtx, stopLoad := context.WithCancel(ctx)
	defer stopLoad()
	var samples map[string][]load.Sample
	loadDone := make(chan struct{})
	go func() {
		samples = load.Run(loadCtx, start, opts.Targets, load.Options{RPS: opts.RPS, Timeout: opts.Timeout})
		close(loadDone)
	}()

	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			faultMu.Lock()
			fault.Watch(watchCtx, k, note, fact)
			faultMu.Unlock()
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	note("phase", fmt.Sprintf("baseline: %s of traffic with nothing broken", opts.Baseline))
	if !sleep(ctx, time.Until(injected)) {
		return nil, ctx.Err()
	}

	// The fault and the watcher share state, so they take turns. The clock
	// every later number is measured from starts once the watcher is out of
	// the way, immediately before the fault.
	faultMu.Lock()
	mu.Lock()
	injected = time.Now()
	result.InjectAt = injected.Sub(start).Seconds()
	mu.Unlock()
	err = fault.Inject(ctx, k)
	faultMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("inject the fault: %w", err)
	}
	note("fault", "fault injected")

	if !sleep(ctx, opts.Observe) {
		return nil, ctx.Err()
	}
	note("phase", "observation window over")

	stopLoad()
	<-loadDone
	stopWatch()
	<-watchDone

	for _, target := range opts.Targets {
		result.Targets = append(result.Targets, TargetResult{
			Name:    target.Name,
			URL:     target.URL,
			Summary: report.Summarise(samples[target.Name], result.InjectAt),
			Samples: samples[target.Name],
		})
	}
	sort.SliceStable(result.Events, func(i, j int) bool { return result.Events[i].T < result.Events[j].T })
	return result, nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// tenths rounds seconds to one decimal place.
func tenths(v float64) float64 { return math.Round(v*10) / 10 }
