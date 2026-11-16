// faildrill runs failure drills against a Tyk gateway on Kubernetes and
// measures what callers saw.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shanurwan/failure-drill/internal/drill"
	"github.com/shanurwan/failure-drill/internal/kube"
	"github.com/shanurwan/failure-drill/internal/load"
	"github.com/shanurwan/failure-drill/internal/preflight"
	"github.com/shanurwan/failure-drill/internal/report"
)

var version = "dev"

const usage = `faildrill breaks a Tyk install on purpose and measures what callers saw.

Usage:
  faildrill preflight [flags]            read the install, change nothing
  faildrill run <drill> [flags]          run one drill while traffic flows
  faildrill export record.json...        bundle records for the replay page
  faildrill version

Drills:
  redis-primary     take the Redis primary away        --fault crash | hang
  gateway-pod       take one gateway pod away          --fault crash | evict
  rolling-restart   restart every gateway pod, as an upgrade does

Run "faildrill preflight -h" or "faildrill run -h" for the flags.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "preflight":
		err = runPreflight(ctx, os.Args[2:])
	case "run":
		err = runDrill(ctx, os.Args[2:])
	case "export":
		err = runExport(os.Args[2:])
	case "version":
		fmt.Println("faildrill", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "faildrill:", err)
		}
		os.Exit(1)
	}
}

// list is a flag that may be given several times.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

type common struct {
	context, namespace, gateway, redisSelector string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.context, "context", "", "kubeconfig context (default: the current one)")
	fs.StringVar(&c.namespace, "n", "", "namespace of the Tyk install (default: the context's)")
	fs.StringVar(&c.gateway, "gateway", "", "gateway Deployment name (default: the one running a tyk-gateway image)")
	fs.StringVar(&c.redisSelector, "redis-selector", "app.kubernetes.io/name=redis", "label selector of the Redis pods")
}

func runPreflight(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	var c common
	c.register(fs)
	asJSON := fs.Bool("json", false, "print the findings as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	k, err := kube.New(c.context, c.namespace)
	if err != nil {
		return err
	}
	gateway, err := drill.FindGateway(ctx, k, c.gateway)
	if err != nil {
		return err
	}
	rep, err := preflight.Run(ctx, k, gateway, c.redisSelector)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	fmt.Printf("gateway    %s  (%s)\n", rep.Gateway, rep.Image)
	fmt.Printf("namespace  %s\n\n", rep.Namespace)
	warnings := 0
	for _, f := range rep.Findings {
		mark := map[preflight.Level]string{preflight.OK: "ok  ", preflight.Warn: "WARN", preflight.Info: "note"}[f.Level]
		fmt.Printf("%s  %s\n", mark, f.What)
		if f.Why != "" {
			fmt.Printf("      %s\n", f.Why)
		}
		if f.Level == preflight.Warn {
			warnings++
			if f.Drill != "" {
				fmt.Printf("      see it happen: faildrill run %s\n", f.Drill)
			}
		}
	}
	fmt.Printf("\n%d warnings. Preflight reads the install; only a drill shows what callers would feel.\n", warnings)
	return nil
}

func runDrill(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: faildrill run <redis-primary|gateway-pod|rolling-restart> [flags]\n\n")
		fs.PrintDefaults()
	}
	var c common
	c.register(fs)
	var urls, headers list
	fs.Var(&urls, "url", "target to call during the drill, as name=URL (repeat for several)")
	fs.Var(&headers, "header", `request header for a target, as name=Key: value (repeat for several)`)
	fault := fs.String("fault", "", "how to break it (see the drill list; default: crash)")
	hold := fs.Duration("hold", 30*time.Second, "redis-primary --fault hang: how long the primary stops answering")
	redisContainer := fs.String("redis-container", "", "container to run redis-cli in (default: the pod's first)")
	redisCLI := fs.String("redis-cli-args", "", `extra redis-cli arguments, for example "-a $PASSWORD"`)
	rps := fs.Int("rps", 50, "requests per second, per target")
	timeout := fs.Duration("timeout", 2*time.Second, "how long a caller waits for one request")
	baseline := fs.Duration("baseline", 15*time.Second, "traffic before the fault")
	observe := fs.Duration("observe", 60*time.Second, "traffic after the fault")
	out := fs.String("out", "drills", "directory for the JSON record and the Markdown report")
	yes := fs.Bool("yes", false, "do not ask before breaking things")

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fs.Usage()
		return flag.ErrHelp
	}
	name := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	targets, err := parseTargets(urls, headers)
	if err != nil {
		return err
	}
	if *rps < 1 {
		return errors.New("--rps must be at least 1")
	}

	var f drill.Fault
	mode := *fault
	if mode == "" {
		mode = "crash"
	}
	switch name {
	case "redis-primary":
		f = &drill.RedisPrimary{Selector: c.redisSelector, Container: *redisContainer, Mode: mode, Hold: *hold, CLIArgs: strings.Fields(*redisCLI)}
	case "gateway-pod":
		f = &drill.GatewayPod{Deployment: c.gateway, Mode: mode}
	case "rolling-restart":
		f = &drill.RollingRestart{Deployment: c.gateway}
	default:
		return fmt.Errorf("unknown drill %q: use redis-primary, gateway-pod or rolling-restart", name)
	}

	k, err := kube.New(c.context, c.namespace)
	if err != nil {
		return err
	}
	if !*yes {
		fmt.Printf("This will break a running system in namespace %q. Type the namespace to continue: ", k.Namespace)
		var answer string
		fmt.Scanln(&answer)
		if answer != k.Namespace {
			return errors.New("not confirmed, nothing was changed")
		}
	}

	var printed bytes.Buffer
	stdout := io.MultiWriter(os.Stdout, &printed)
	result, err := drill.Run(ctx, k, f, drill.Options{
		Targets: targets, RPS: *rps, Timeout: *timeout, Baseline: *baseline, Observe: *observe, Log: stdout,
	})
	if err != nil {
		return err
	}

	rows := make([]report.Row, len(result.Targets))
	for i, target := range result.Targets {
		rows[i] = report.Row{Name: target.Name, Summary: target.Summary}
	}
	report.Text(stdout, rows, result.Facts)
	result.Output = printed.String()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	base := filepath.Join(*out, fmt.Sprintf("%s-%s-%s", result.Drill, result.Fault, result.Started.Format("20060102-150405")))
	record, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".json", record, 0o644); err != nil {
		return err
	}
	md, err := os.Create(base + ".md")
	if err != nil {
		return err
	}
	defer md.Close()
	events := make([]string, len(result.Events))
	for i, e := range result.Events {
		events[i] = fmt.Sprintf("%+7.1fs  %s", e.T, e.Text)
	}
	context := fmt.Sprintf("Run on %s in namespace `%s`: %d requests per second per target, %s caller timeout, %s of baseline and %s of observation.",
		result.Started.Format("2 January 2006 at 15:04 UTC"), result.Namespace, result.RPS, *timeout, *baseline, *observe)
	report.Markdown(md, fmt.Sprintf("Drill: %s (%s)", result.Drill, result.Fault), context, rows, result.Facts, events)

	fmt.Printf("\nrecord  %s.json\nreport  %s.md\n", base, base)
	return nil
}

func parseTargets(urls, headers list) ([]load.Target, error) {
	if len(urls) == 0 {
		return nil, errors.New("give at least one --url name=URL to call during the drill")
	}
	var targets []load.Target
	index := map[string]int{}
	for _, entry := range urls {
		name, url, ok := strings.Cut(entry, "=")
		if !ok || name == "" || !strings.HasPrefix(url, "http") {
			return nil, fmt.Errorf("--url %q: want name=http(s)://...", entry)
		}
		if _, dup := index[name]; dup {
			return nil, fmt.Errorf("--url: target %q given twice", name)
		}
		index[name] = len(targets)
		targets = append(targets, load.Target{Name: name, URL: url, Header: http.Header{}})
	}
	for _, entry := range headers {
		name, header, ok := strings.Cut(entry, "=")
		key, value, ok2 := strings.Cut(header, ":")
		i, known := index[name]
		if !ok || !ok2 || !known {
			return nil, fmt.Errorf(`--header %q: want name=Key: value, for a name given with --url`, entry)
		}
		targets[i].Header.Add(strings.TrimSpace(key), strings.TrimSpace(value))
	}
	return targets, nil
}
