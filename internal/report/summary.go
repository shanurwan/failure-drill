// Package report turns the samples of a drill into numbers a person can act on.
package report

import (
	"math"
	"sort"
	"strconv"

	"github.com/shanurwan/failure-drill/internal/load"
)

// Summary describes one target over one drill. Times are in seconds relative
// to the moment the fault was injected, so negative means before it.
type Summary struct {
	Requests int `json:"requests"`
	Failed   int `json:"failed"`
	// FailedBefore counts failures before the fault: anything above zero means
	// the baseline was not clean and the drill says less than it seems to.
	FailedBefore int            `json:"failed_before"`
	ByOutcome    map[string]int `json:"by_outcome"`

	// FirstFailure and LastFailure bound the failures after the fault.
	FirstFailure *float64 `json:"first_failure_s"`
	LastFailure  *float64 `json:"last_failure_s"`
	// ImpactSeconds is LastFailure - FirstFailure: how long callers kept
	// seeing errors, counting the quiet stretches in between.
	ImpactSeconds float64 `json:"impact_s"`
	// LongestOutage is the longest stretch in which every request failed.
	LongestOutage float64 `json:"longest_outage_s"`

	BaselineP50 float64 `json:"baseline_p50_ms"`
	BaselineP99 float64 `json:"baseline_p99_ms"`
	AfterP50    float64 `json:"after_p50_ms"`
	AfterP99    float64 `json:"after_p99_ms"`
	AfterMax    float64 `json:"after_max_ms"`

	// Seconds holds one bucket per second of the run, for plotting.
	Seconds []Second `json:"seconds"`
}

// Second is the traffic sent during one second of the run.
type Second struct {
	T     int     `json:"t"`
	OK    int     `json:"ok"`
	Fail  int     `json:"fail"`
	P99Ms float64 `json:"p99_ms"`
}

// Summarise computes the Summary of samples for a fault injected at injectAt
// (seconds since the run started).
func Summarise(samples []load.Sample, injectAt float64) Summary {
	s := Summary{Requests: len(samples), ByOutcome: map[string]int{}}
	var before, after []float64
	var outageStart *float64
	lastSent := injectAt

	type bucket struct {
		ok, fail int
		ms       []float64
	}
	buckets := map[int]*bucket{}

	for _, sample := range samples {
		rel := sample.T - injectAt
		second := int(math.Floor(rel))
		b := buckets[second]
		if b == nil {
			b = &bucket{}
			buckets[second] = b
		}
		b.ms = append(b.ms, sample.Ms)
		s.ByOutcome[outcome(sample)]++

		if sample.OK() {
			b.ok++
			if rel < 0 {
				before = append(before, sample.Ms)
			} else {
				after = append(after, sample.Ms)
				if outageStart != nil {
					s.LongestOutage = math.Max(s.LongestOutage, rel-*outageStart)
					outageStart = nil
				}
			}
			lastSent = sample.T
			continue
		}

		b.fail++
		s.Failed++
		if rel < 0 {
			s.FailedBefore++
			continue
		}
		after = append(after, sample.Ms)
		if s.FirstFailure == nil {
			first := rel
			s.FirstFailure = &first
		}
		last := rel
		s.LastFailure = &last
		if outageStart == nil {
			start := rel
			outageStart = &start
		}
		lastSent = sample.T
	}
	if outageStart != nil {
		s.LongestOutage = math.Max(s.LongestOutage, lastSent-injectAt-*outageStart)
	}
	if s.FirstFailure != nil {
		s.ImpactSeconds = *s.LastFailure - *s.FirstFailure
	}

	s.BaselineP50, s.BaselineP99 = percentile(before, 0.50), percentile(before, 0.99)
	s.AfterP50, s.AfterP99, s.AfterMax = percentile(after, 0.50), percentile(after, 0.99), percentile(after, 1)

	seconds := make([]int, 0, len(buckets))
	for second := range buckets {
		seconds = append(seconds, second)
	}
	sort.Ints(seconds)
	for _, second := range seconds {
		b := buckets[second]
		s.Seconds = append(s.Seconds, Second{T: second, OK: b.ok, Fail: b.fail, P99Ms: round(percentile(b.ms, 0.99))})
	}

	s.ImpactSeconds, s.LongestOutage = round(s.ImpactSeconds), round(s.LongestOutage)
	s.BaselineP50, s.BaselineP99 = round(s.BaselineP50), round(s.BaselineP99)
	s.AfterP50, s.AfterP99, s.AfterMax = round(s.AfterP50), round(s.AfterP99), round(s.AfterMax)
	if s.FirstFailure != nil {
		*s.FirstFailure, *s.LastFailure = round(*s.FirstFailure), round(*s.LastFailure)
	}
	return s
}

func outcome(sample load.Sample) string {
	// The exact status matters: a 403 for a valid key and a 503 are different
	// things to tell a caller.
	if sample.Err != "" {
		return sample.Err
	}
	return strconv.Itoa(sample.Status)
}

// percentile returns the p-th percentile (0..1) by the nearest-rank method, 0
// for no values.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	return sorted[rank]
}

func round(v float64) float64 { return math.Round(v*100) / 100 }
