package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shanurwan/failure-drill/internal/load"
)

// traffic builds one sample every 100 ms for the given number of seconds.
// status is asked for each one by its index, in tenths of a second, so the
// tests can compare whole numbers.
func traffic(seconds int, status func(tenths int) (int, string)) []load.Sample {
	var samples []load.Sample
	for i := 0; i < seconds*10; i++ {
		code, err := status(i)
		samples = append(samples, load.Sample{T: float64(i) / 10, Status: code, Err: err, Ms: 2})
	}
	return samples
}

func TestSummariseCleanRun(t *testing.T) {
	s := Summarise(traffic(20, func(int) (int, string) { return 200, "" }), 10)

	if s.Requests != 200 || s.Failed != 0 {
		t.Fatalf("requests=%d failed=%d, want 200 and 0", s.Requests, s.Failed)
	}
	if s.FirstFailure != nil || s.LastFailure != nil {
		t.Fatalf("a clean run has no failure window, got %v..%v", s.FirstFailure, s.LastFailure)
	}
	if s.ImpactSeconds != 0 || s.LongestOutage != 0 {
		t.Fatalf("impact=%v longest=%v, want 0", s.ImpactSeconds, s.LongestOutage)
	}
	if got := s.ByOutcome["200"]; got != 200 {
		t.Fatalf("by_outcome[200]=%d, want 200", got)
	}
	if first, last := s.Seconds[0].T, s.Seconds[len(s.Seconds)-1].T; first != -10 || last != 9 {
		t.Fatalf("seconds run from %d to %d, want -10 to 9 (relative to the fault)", first, last)
	}
}

// Failures from 2 s to 5 s after the fault, with one good answer in the middle:
// the impact is the whole stretch, the longest gap only the longer half.
func TestSummariseOutageWithARecoveryInTheMiddle(t *testing.T) {
	samples := traffic(30, func(tenths int) (int, string) {
		rel := tenths - 100
		switch {
		case rel >= 20 && rel < 30:
			return 403, ""
		case rel >= 31 && rel < 50:
			return 0, "timeout"
		}
		return 200, ""
	})
	s := Summarise(samples, 10)

	if s.Failed != 29 || s.FailedBefore != 0 {
		t.Fatalf("failed=%d before=%d, want 29 and 0", s.Failed, s.FailedBefore)
	}
	if *s.FirstFailure != 2 || *s.LastFailure != 4.9 {
		t.Fatalf("failure window %v..%v, want 2..4.9", *s.FirstFailure, *s.LastFailure)
	}
	if s.ImpactSeconds != 2.9 {
		t.Fatalf("impact=%v, want 2.9", s.ImpactSeconds)
	}
	if s.LongestOutage != 1.9 {
		t.Fatalf("longest gap=%v, want 1.9 (3.1 to 5.0)", s.LongestOutage)
	}
	if s.ByOutcome["403"] != 10 || s.ByOutcome["timeout"] != 19 {
		t.Fatalf("by_outcome=%v, want 10 x 403 and 19 x timeout", s.ByOutcome)
	}
}

func TestSummariseCountsFailuresBeforeTheFaultSeparately(t *testing.T) {
	samples := traffic(20, func(tenths int) (int, string) {
		if tenths < 10 {
			return 502, ""
		}
		return 200, ""
	})
	s := Summarise(samples, 10)

	if s.FailedBefore != 10 || s.Failed != 10 {
		t.Fatalf("failed=%d before=%d, want 10 and 10", s.Failed, s.FailedBefore)
	}
	if s.FirstFailure != nil {
		t.Fatalf("failures before the fault must not open a failure window, got %v", *s.FirstFailure)
	}

	var out bytes.Buffer
	Text(&out, []Row{{Name: "api", Summary: s}}, nil)
	if !strings.Contains(out.String(), "the baseline was not clean") {
		t.Fatalf("the text report must flag a dirty baseline:\n%s", out.String())
	}
}

func TestPercentileNearestRank(t *testing.T) {
	values := []float64{5, 1, 4, 2, 3}
	for p, want := range map[float64]float64{0.5: 3, 0.99: 5, 1: 5, 0.2: 1} {
		if got := percentile(values, p); got != want {
			t.Errorf("percentile(%v)=%v, want %v", p, got, want)
		}
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("percentile of nothing=%v, want 0", got)
	}
}
