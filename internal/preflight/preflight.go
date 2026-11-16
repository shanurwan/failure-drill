// Package preflight reads a Tyk install and says, before anything is broken,
// which failures it is not prepared for. It changes nothing.
package preflight

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/shanurwan/failure-drill/internal/kube"
)

// Level says how much a finding matters.
type Level string

const (
	OK   Level = "ok"
	Warn Level = "warn"
	Info Level = "info"
)

// Finding is one thing preflight looked at.
type Finding struct {
	ID    string `json:"id"`
	Level Level  `json:"level"`
	// What was found, as a fact about this install.
	What string `json:"what"`
	// Why it matters when something fails. Empty for findings that are fine.
	Why string `json:"why,omitempty"`
	// Drill names the drill that shows the consequence.
	Drill string `json:"drill,omitempty"`
}

// Report is everything preflight found.
type Report struct {
	Namespace string    `json:"namespace"`
	Gateway   string    `json:"gateway"`
	Image     string    `json:"image"`
	Findings  []Finding `json:"findings"`
}

// Run inspects the gateway Deployment and, when redisSelector matches pods,
// the Redis it runs beside.
func Run(ctx context.Context, k *kube.Client, gateway *appsv1.Deployment, redisSelector string) (*Report, error) {
	spec := gateway.Spec.Template.Spec
	container := gatewayContainer(spec)
	report := &Report{Namespace: k.Namespace, Gateway: gateway.Name, Image: container.Image}
	add := func(f Finding) { report.Findings = append(report.Findings, f) }

	selector := labels.SelectorFromSet(gateway.Spec.Selector.MatchLabels)
	pods, err := k.Pods(ctx, selector.String())
	if err != nil {
		return nil, err
	}

	// Replicas.
	replicas := int32(1)
	if gateway.Spec.Replicas != nil {
		replicas = *gateway.Spec.Replicas
	}
	if replicas < 2 {
		add(Finding{ID: "replicas", Level: Warn, Drill: "gateway-pod",
			What: fmt.Sprintf("%d gateway replica", replicas),
			Why:  "Every restart, crash or node drain is a full outage until the pod is ready again."})
	} else {
		add(Finding{ID: "replicas", Level: OK, What: fmt.Sprintf("%d gateway replicas", replicas)})
	}

	// Where the pods actually are.
	nodes := map[string]int{}
	for _, pod := range pods {
		if pod.Spec.NodeName != "" {
			nodes[pod.Spec.NodeName]++
		}
	}
	spread := len(spec.TopologySpreadConstraints) > 0 ||
		(spec.Affinity != nil && spec.Affinity.PodAntiAffinity != nil)
	switch {
	case replicas >= 2 && len(nodes) == 1:
		add(Finding{ID: "spread", Level: Warn, Drill: "gateway-pod",
			What: fmt.Sprintf("all %d gateway pods run on one node (%s)", len(pods), firstKey(nodes)),
			Why:  "Losing that node takes every gateway with it. Replicas only help when they are apart."})
	case replicas >= 2 && !spread:
		add(Finding{ID: "spread", Level: Info,
			What: fmt.Sprintf("gateway pods are on %d nodes today, but nothing keeps them apart", len(nodes)),
			Why:  "No topology spread or anti-affinity is set, so a reschedule can put them on one node."})
	case replicas >= 2:
		add(Finding{ID: "spread", Level: OK, What: fmt.Sprintf("gateway pods are spread over %d nodes by rule", len(nodes))})
	}

	// Disruption budget.
	budgets, err := k.Set.PolicyV1().PodDisruptionBudgets(k.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pod disruption budgets: %w", err)
	}
	covered := false
	for _, budget := range budgets.Items {
		s, err := metav1.LabelSelectorAsSelector(budget.Spec.Selector)
		if err == nil && !s.Empty() && s.Matches(labels.Set(gateway.Spec.Template.Labels)) {
			covered = true
		}
	}
	if covered {
		add(Finding{ID: "pdb", Level: OK, What: "a PodDisruptionBudget covers the gateway"})
	} else {
		add(Finding{ID: "pdb", Level: Warn, Drill: "gateway-pod",
			What: "no PodDisruptionBudget covers the gateway",
			Why:  "A node drain or cluster upgrade may evict every gateway pod at the same time."})
	}

	// Stopping.
	hasPreStop := container.Lifecycle != nil && container.Lifecycle.PreStop != nil
	if hasPreStop {
		add(Finding{ID: "prestop", Level: OK, What: "the gateway has a preStop hook"})
	} else {
		add(Finding{ID: "prestop", Level: Warn, Drill: "rolling-restart",
			What: "the gateway has no preStop hook",
			Why:  "A stopping pod gets SIGTERM while the Service is still sending it requests; removing it from the endpoints takes a few seconds to reach every node."})
	}

	// Readiness.
	if probe := container.ReadinessProbe; probe == nil {
		add(Finding{ID: "readiness", Level: Warn, Drill: "rolling-restart",
			What: "the gateway has no readiness probe",
			Why:  "A new pod receives traffic before it has loaded its APIs."})
	} else {
		seconds := probe.PeriodSeconds * probe.FailureThreshold
		level, why := OK, ""
		if seconds > 15 {
			level = Info
			why = "A pod that stops being able to serve keeps receiving traffic for that long before Kubernetes takes it out."
		}
		add(Finding{ID: "readiness", Level: level, Drill: "gateway-pod",
			What: fmt.Sprintf("readiness probe %s every %ds, unready after %d failures (%ds)", probePath(probe), probe.PeriodSeconds, probe.FailureThreshold, seconds),
			Why:  why})
	}

	// Restart strategy.
	if ru := gateway.Spec.Strategy.RollingUpdate; ru != nil && ru.MaxUnavailable != nil && ru.MaxUnavailable.String() != "0" {
		level := Info
		if replicas <= 2 {
			level = Warn
		}
		add(Finding{ID: "rollout", Level: level, Drill: "rolling-restart",
			What: fmt.Sprintf("rolling update allows %s unavailable of %d", ru.MaxUnavailable.String(), replicas),
			Why:  "During every upgrade the gateway runs below strength; with few replicas that is a large share of capacity."})
	}

	// Resources.
	if _, ok := container.Resources.Requests[corev1.ResourceCPU]; !ok {
		add(Finding{ID: "resources", Level: Info,
			What: "the gateway has no CPU request",
			Why:  "The scheduler may pack it onto a busy node, and it is first to be starved under pressure."})
	}

	// Redis, as the gateway is configured to reach it.
	env := envMap(container)
	addrs := splitList(env["TYK_GW_STORAGE_ADDRS"])
	if len(addrs) == 0 && env["TYK_GW_STORAGE_HOST"] != "" {
		addrs = []string{env["TYK_GW_STORAGE_HOST"]}
	}
	sentinel := env["TYK_GW_STORAGE_MASTERNAME"] != ""
	cluster := strings.EqualFold(env["TYK_GW_STORAGE_ENABLECLUSTER"], "true")
	switch {
	case sentinel:
		add(Finding{ID: "redis-mode", Level: OK, Drill: "redis-primary",
			What: fmt.Sprintf("the gateway finds Redis through Sentinel (master name %q, %s)", env["TYK_GW_STORAGE_MASTERNAME"], strings.Join(addrs, ", "))})
	case cluster:
		add(Finding{ID: "redis-mode", Level: OK, Drill: "redis-primary",
			What: fmt.Sprintf("the gateway uses Redis Cluster (%s)", strings.Join(addrs, ", "))})
	case len(addrs) > 0:
		add(Finding{ID: "redis-mode", Level: Warn, Drill: "redis-primary",
			What: fmt.Sprintf("the gateway is given one Redis address (%s) with no Sentinel or Cluster mode", strings.Join(addrs, ", ")),
			Why:  "If that Redis is replaced by another one, the gateway has no way to find the new primary by itself."})
	default:
		add(Finding{ID: "redis-mode", Level: Info,
			What: "the Redis address is not set through TYK_GW_STORAGE_* environment variables",
			Why:  "It is probably in a mounted tyk.conf, which preflight does not read."})
	}

	// Redis itself.
	if redisSelector != "" {
		redisPods, err := k.Pods(ctx, redisSelector)
		if err != nil {
			return nil, err
		}
		if len(redisPods) == 0 {
			add(Finding{ID: "redis-pods", Level: Info,
				What: fmt.Sprintf("no pods match --redis-selector %q in this namespace", redisSelector),
				Why:  "Redis may run elsewhere or be a managed service; the redis-primary drill needs pods it can reach."})
		} else {
			persistent := 0
			for _, pod := range redisPods {
				if hasPersistentVolume(pod.Spec) {
					persistent++
				}
			}
			switch {
			case len(redisPods) == 1:
				add(Finding{ID: "redis-pods", Level: Warn, Drill: "redis-primary",
					What: "Redis is a single pod",
					Why:  "There is no replica to promote: any Redis failure lasts until that pod is back."})
			default:
				add(Finding{ID: "redis-pods", Level: OK, What: fmt.Sprintf("%d Redis pods", len(redisPods))})
			}
			if persistent < len(redisPods) {
				add(Finding{ID: "redis-storage", Level: Warn, Drill: "redis-primary",
					What: fmt.Sprintf("%d of %d Redis pods have no persistent volume", len(redisPods)-persistent, len(redisPods)),
					Why:  "A restarted primary comes back empty. API keys, quotas and OAuth tokens live in Redis, and replicas then copy the empty state."})
			} else {
				add(Finding{ID: "redis-storage", Level: OK, What: "every Redis pod has a persistent volume"})
			}
		}
	}

	return report, nil
}

func gatewayContainer(spec corev1.PodSpec) corev1.Container {
	for _, container := range spec.Containers {
		if strings.Contains(container.Image, "tyk-gateway") {
			return container
		}
	}
	return spec.Containers[0]
}

func envMap(container corev1.Container) map[string]string {
	env := map[string]string{}
	for _, e := range container.Env {
		env[e.Name] = e.Value
	}
	return env
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func probePath(probe *corev1.Probe) string {
	if probe.HTTPGet != nil {
		return probe.HTTPGet.Path
	}
	if probe.Exec != nil {
		return "exec"
	}
	return "tcp"
}

func hasPersistentVolume(spec corev1.PodSpec) bool {
	for _, volume := range spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}

func firstKey(m map[string]int) string {
	for key := range m {
		return key
	}
	return ""
}
