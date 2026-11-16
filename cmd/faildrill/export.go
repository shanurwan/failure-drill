package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/shanurwan/failure-drill/internal/drill"
)

// bundle is what the replay site loads: every record without its raw samples,
// which the per-second buckets already summarise.
type bundle struct {
	// Usage is this tool's own usage text, Preflight and Pods the recorded
	// output of `faildrill preflight` and `kubectl get pods` on the same lab.
	Usage     string          `json:"usage"`
	Preflight string          `json:"preflight"`
	Pods      string          `json:"pods"`
	Runs      []*drill.Result `json:"runs"`
}

// runExport bundles drill records into one file for the replay site.
func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("o", "site/data/runs.json", "file to write")
	preflight := fs.String("preflight", "", "file holding the text output of faildrill preflight")
	pods := fs.String("pods", "", "file holding the output of kubectl get pods")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: faildrill export [flags] record.json...\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return flag.ErrHelp
	}

	b := bundle{Usage: usage}
	for path, into := range map[string]*string{*preflight: &b.Preflight, *pods: &b.Pods} {
		if path == "" {
			continue
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		*into = string(text)
	}
	for _, path := range fs.Args() {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var result drill.Result
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for i := range result.Targets {
			result.Targets[i].Samples = nil
		}
		for i := range result.Events {
			result.Events[i].T = math.Round(result.Events[i].T*10) / 10
		}
		b.Runs = append(b.Runs, &result)
	}
	sort.SliceStable(b.Runs, func(i, j int) bool { return b.Runs[i].Started.Before(b.Runs[j].Started) })

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("%d runs -> %s\n", len(b.Runs), *out)
	return nil
}
