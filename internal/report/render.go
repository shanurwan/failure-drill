package report

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Row is one target's summary with its name, for rendering.
type Row struct {
	Name    string
	Summary Summary
}

// Text writes the result table the CLI prints at the end of a drill.
func Text(w io.Writer, rows []Row, facts map[string]string) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-8s %9s %8s %10s %12s %10s %10s\n", "TARGET", "REQUESTS", "FAILED", "IMPACT", "LONGEST GAP", "P99 BEFORE", "P99 AFTER")
	for _, row := range rows {
		s := row.Summary
		fmt.Fprintf(w, "%-8s %9d %8d %10s %12s %10s %10s\n",
			row.Name, s.Requests, s.Failed, seconds(s.ImpactSeconds, s.Failed-s.FailedBefore), seconds(s.LongestOutage, s.Failed-s.FailedBefore),
			ms(s.BaselineP99), ms(s.AfterP99))
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		s := row.Summary
		fmt.Fprintf(w, "%-8s %s\n", row.Name, outcomes(s.ByOutcome))
		if s.FirstFailure != nil {
			fmt.Fprintf(w, "%-8s failures from %+.1fs to %+.1fs after the fault\n", "", *s.FirstFailure, *s.LastFailure)
		}
		if s.FailedBefore > 0 {
			fmt.Fprintf(w, "%-8s %d requests failed before the fault: the baseline was not clean\n", "", s.FailedBefore)
		}
	}
	if len(facts) > 0 {
		fmt.Fprintln(w)
		keys := make([]string, 0, len(facts))
		for key := range facts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(w, "%-24s %s\n", key, facts[key])
		}
	}
}

// Markdown writes the same result as a report to keep or attach to an issue.
func Markdown(w io.Writer, title, context string, rows []Row, facts map[string]string, events []string) {
	fmt.Fprintf(w, "# %s\n\n%s\n\n", title, context)
	fmt.Fprintln(w, "| Target | Requests | Failed | Impact | Longest gap | p99 before | p99 after | Slowest after |")
	fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|---:|---:|")
	for _, row := range rows {
		s := row.Summary
		fmt.Fprintf(w, "| %s | %d | %d | %s | %s | %s | %s | %s |\n",
			row.Name, s.Requests, s.Failed, seconds(s.ImpactSeconds, s.Failed-s.FailedBefore), seconds(s.LongestOutage, s.Failed-s.FailedBefore),
			ms(s.BaselineP99), ms(s.AfterP99), ms(s.AfterMax))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Impact is the time from the first failed request to the last. Longest gap is the longest stretch in which every request failed.")
	fmt.Fprintln(w)
	for _, row := range rows {
		fmt.Fprintf(w, "- **%s**: %s\n", row.Name, outcomes(row.Summary.ByOutcome))
	}
	if len(facts) > 0 {
		fmt.Fprintf(w, "\n## What the system did\n\n")
		keys := make([]string, 0, len(facts))
		for key := range facts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(w, "- `%s`: %s\n", key, facts[key])
		}
	}
	if len(events) > 0 {
		fmt.Fprintf(w, "\n## Timeline\n\nSeconds are relative to the fault.\n\n```\n%s\n```\n", strings.Join(events, "\n"))
	}
}

func outcomes(by map[string]int) string {
	keys := make([]string, 0, len(by))
	for key := range by {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		label := key
		if _, err := strconv.Atoi(key); err == nil {
			label = "HTTP " + key
		}
		parts[i] = fmt.Sprintf("%d × %s", by[key], label)
	}
	return strings.Join(parts, ", ")
}

func seconds(v float64, failedAfter int) string {
	if failedAfter <= 0 {
		return "none"
	}
	return fmt.Sprintf("%.1fs", v)
}

func ms(v float64) string {
	if v >= 100 {
		return fmt.Sprintf("%.0fms", v)
	}
	return fmt.Sprintf("%.1fms", v)
}
