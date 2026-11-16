package drill

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shanurwan/failure-drill/internal/kube"
)

// RedisPrimary takes the Redis primary away from the gateway.
//
// Two ways to do it, because they are different failures:
//
//   - crash: the pod is killed without warning. Kubernetes restarts it within
//     seconds, usually before Sentinel has decided it is down, so this tests a
//     restart, not a failover.
//   - hang: the primary stops answering for Hold but stays up (CLIENT PAUSE),
//     the way a frozen node or a stalled disk looks from outside. Sentinel has
//     to notice and promote a replica.
type RedisPrimary struct {
	Selector  string
	Container string
	Mode      string
	Hold      time.Duration
	CLIArgs   []string

	primary string
	// last is what each pod answered the previous time Watch looked.
	last map[string]string
	// promotedAt and returnedAt are set once, the first time each is seen.
	promoted, returned bool
	injectedAt         time.Time
}

func (r *RedisPrimary) Name() string { return "redis-primary" }
func (r *RedisPrimary) Kind() string { return r.Mode }

func (r *RedisPrimary) Prepare(ctx context.Context, k *kube.Client) (string, error) {
	roles, err := r.roles(ctx, k)
	if err != nil {
		return "", err
	}
	if len(roles) == 0 {
		return "", fmt.Errorf("no pods match --redis-selector %q in namespace %s", r.Selector, k.Namespace)
	}
	var primaries, replicas []string
	for pod, role := range roles {
		switch role {
		case "master":
			primaries = append(primaries, pod)
		case "slave":
			replicas = append(replicas, pod)
		}
	}
	sort.Strings(primaries)
	if len(primaries) != 1 {
		return "", fmt.Errorf("expected exactly one Redis primary before the drill, found %d (%s): fix replication first", len(primaries), describe(roles))
	}
	r.primary = primaries[0]
	r.last = roles

	switch r.Mode {
	case "crash":
		return fmt.Sprintf("kill %s (primary, %d replicas) without warning and let Kubernetes restart it", r.primary, len(replicas)), nil
	case "hang":
		if len(replicas) == 0 {
			return "", fmt.Errorf("%s has no replicas: a hang would only be an outage, there is nothing to fail over to", r.primary)
		}
		return fmt.Sprintf("make %s (primary, %d replicas) stop answering for %s", r.primary, len(replicas), r.Hold), nil
	default:
		return "", fmt.Errorf("unknown --fault %q for redis-primary: use crash or hang", r.Mode)
	}
}

func (r *RedisPrimary) Inject(ctx context.Context, k *kube.Client) error {
	r.injectedAt = time.Now()
	if r.Mode == "crash" {
		zero := int64(0)
		return k.DeletePod(ctx, r.primary, &zero)
	}
	args := append(r.cli(), "client", "pause", fmt.Sprint(r.Hold.Milliseconds()), "all")
	out, err := k.Exec(ctx, r.primary, r.Container, 5*time.Second, args...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "OK" {
		return fmt.Errorf("CLIENT PAUSE on %s answered %q", r.primary, strings.TrimSpace(out))
	}
	return nil
}

func (r *RedisPrimary) Watch(ctx context.Context, k *kube.Client, note func(kind, text string), fact func(key, value string)) {
	roles, err := r.roles(ctx, k)
	if err != nil || ctx.Err() != nil {
		return
	}
	fact("primary_before", r.primary)

	pods := make([]string, 0, len(roles))
	for pod := range roles {
		pods = append(pods, pod)
	}
	sort.Strings(pods)

	for _, pod := range pods {
		role, was := roles[pod], r.last[pod]
		if role == was {
			continue
		}
		switch role {
		case "master":
			if pod == r.primary {
				note("redis", fmt.Sprintf("%s is answering again, still as primary", pod))
			} else {
				note("redis", fmt.Sprintf("%s promoted to primary", pod))
				if !r.promoted && !r.injectedAt.IsZero() {
					r.promoted = true
					fact("primary_after", pod)
					fact("promotion_s", fmt.Sprintf("%.1f", time.Since(r.injectedAt).Seconds()))
				}
			}
		case "slave":
			if pod == r.primary {
				note("redis", fmt.Sprintf("%s is back, now as a replica", pod))
				if !r.returned {
					r.returned = true
					fact("old_primary_returned_s", fmt.Sprintf("%.1f", time.Since(r.injectedAt).Seconds()))
				}
			} else {
				note("redis", fmt.Sprintf("%s is a replica", pod))
			}
		default:
			note("redis", fmt.Sprintf("%s stopped answering", pod))
		}
	}
	r.last = roles

	if !r.injectedAt.IsZero() && !r.promoted {
		fact("primary_after", r.primary)
	}
}

// roles asks every Redis pod what it is: "master", "slave", or "" when it does
// not answer within a second.
func (r *RedisPrimary) roles(ctx context.Context, k *kube.Client) (map[string]string, error) {
	pods, err := k.Pods(ctx, r.Selector)
	if err != nil {
		return nil, err
	}
	type answer struct{ pod, role string }
	answers := make(chan answer, len(pods))
	for _, pod := range pods {
		go func(name string, ready bool) {
			role := ""
			if ready {
				out, err := k.Exec(ctx, name, r.Container, time.Second, append(r.cli(), "role")...)
				if err == nil {
					role = strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
				}
			}
			answers <- answer{name, role}
		}(pod.Name, pod.DeletionTimestamp == nil && pod.Status.Phase == "Running")
	}
	roles := make(map[string]string, len(pods))
	for range pods {
		a := <-answers
		roles[a.pod] = a.role
	}
	return roles, nil
}

func (r *RedisPrimary) cli() []string {
	return append([]string{"redis-cli"}, r.CLIArgs...)
}

func describe(roles map[string]string) string {
	parts := make([]string, 0, len(roles))
	for pod, role := range roles {
		if role == "" {
			role = "no answer"
		}
		parts = append(parts, pod+"="+role)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
