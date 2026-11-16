package drill

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/shanurwan/failure-drill/internal/kube"
)

// FindGateway returns the Deployment whose pods run the Tyk gateway. An empty
// name means "the only one in the namespace".
func FindGateway(ctx context.Context, k *kube.Client, name string) (*appsv1.Deployment, error) {
	deployments, err := k.Set.AppsV1().Deployments(k.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	var found []*appsv1.Deployment
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if name != "" {
			if d.Name == name {
				return d, nil
			}
			continue
		}
		if IsGateway(d.Spec.Template.Spec) {
			found = append(found, d)
		}
	}
	switch {
	case name != "":
		return nil, fmt.Errorf("no deployment %q in namespace %s", name, k.Namespace)
	case len(found) == 0:
		return nil, fmt.Errorf("no deployment in namespace %s runs a tyk-gateway image: name it with --gateway", k.Namespace)
	case len(found) > 1:
		names := make([]string, len(found))
		for i, d := range found {
			names[i] = d.Name
		}
		return nil, fmt.Errorf("several gateway deployments in namespace %s (%s): pick one with --gateway", k.Namespace, strings.Join(names, ", "))
	}
	return found[0], nil
}

// IsGateway reports whether a pod spec runs the Tyk gateway.
func IsGateway(spec corev1.PodSpec) bool {
	for _, container := range spec.Containers {
		if strings.Contains(container.Image, "tyk-gateway") {
			return true
		}
	}
	return false
}

// readiness tracks which pods of a Deployment are ready, and reports changes.
type readiness struct {
	selector string
	last     map[string]bool
}

func (r *readiness) watch(ctx context.Context, k *kube.Client, note func(kind, text string)) (ready, total int) {
	pods, err := k.Pods(ctx, r.selector)
	if err != nil || ctx.Err() != nil {
		return -1, -1
	}
	now := make(map[string]bool, len(pods))
	for _, pod := range pods {
		now[pod.Name] = kube.Ready(pod)
		if now[pod.Name] {
			ready++
		}
	}
	names := make([]string, 0, len(now)+len(r.last))
	for name := range now {
		names = append(names, name)
	}
	for name := range r.last {
		if _, still := now[name]; !still {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if r.last != nil {
		for _, name := range names {
			isReady, exists := now[name]
			wasReady, existed := r.last[name]
			switch {
			case !existed:
				note("pod", fmt.Sprintf("%s created", name))
				if isReady {
					note("pod", fmt.Sprintf("%s ready", name))
				}
			case !exists:
				note("pod", fmt.Sprintf("%s gone", name))
			case isReady && !wasReady:
				note("pod", fmt.Sprintf("%s ready", name))
			case !isReady && wasReady:
				note("pod", fmt.Sprintf("%s no longer ready", name))
			}
		}
	}
	r.last = now
	return ready, len(pods)
}

// GatewayPod takes one gateway pod away.
//
//   - crash: killed without warning, as when its node dies.
//   - evict: deleted with its grace period, as in a node drain or a scale-down.
type GatewayPod struct {
	Deployment string
	Mode       string

	deployment *appsv1.Deployment
	victim     string
	pods       readiness
	want       int
	injectedAt time.Time
	recovered  bool
}

func (g *GatewayPod) Name() string { return "gateway-pod" }
func (g *GatewayPod) Kind() string { return g.Mode }

func (g *GatewayPod) Prepare(ctx context.Context, k *kube.Client) (string, error) {
	if g.Mode != "crash" && g.Mode != "evict" {
		return "", fmt.Errorf("unknown --fault %q for gateway-pod: use crash or evict", g.Mode)
	}
	d, err := FindGateway(ctx, k, g.Deployment)
	if err != nil {
		return "", err
	}
	g.deployment = d
	g.pods.selector = labels.SelectorFromSet(d.Spec.Selector.MatchLabels).String()
	pods, err := k.Pods(ctx, g.pods.selector)
	if err != nil {
		return "", err
	}
	var ready []string
	for _, pod := range pods {
		if kube.Ready(pod) {
			ready = append(ready, pod.Name)
		}
	}
	sort.Strings(ready)
	if len(ready) == 0 {
		return "", fmt.Errorf("deployment %s has no ready pods: nothing to drill", d.Name)
	}
	g.victim, g.want = ready[0], len(ready)
	how := "kill it without warning"
	if g.Mode == "evict" {
		how = "delete it with its grace period, as a node drain would"
	}
	return fmt.Sprintf("%s has %d ready pods; take %s away (%s)", d.Name, len(ready), g.victim, how), nil
}

func (g *GatewayPod) Inject(ctx context.Context, k *kube.Client) error {
	g.injectedAt = time.Now()
	var grace *int64
	if g.Mode == "crash" {
		zero := int64(0)
		grace = &zero
	}
	return k.DeletePod(ctx, g.victim, grace)
}

func (g *GatewayPod) Watch(ctx context.Context, k *kube.Client, note func(kind, text string), fact func(key, value string)) {
	ready, _ := g.pods.watch(ctx, k, note)
	fact("deployment", g.deployment.Name)
	fact("pod_removed", g.victim)
	fact("ready_before", fmt.Sprint(g.want))
	if g.injectedAt.IsZero() || g.recovered || ready < 0 {
		return
	}
	// Recovered means the full count is ready again without the removed pod.
	if _, stillThere := g.pods.last[g.victim]; !stillThere && ready >= g.want {
		g.recovered = true
		fact("full_strength_s", fmt.Sprintf("%.1f", time.Since(g.injectedAt).Seconds()))
		note("pod", fmt.Sprintf("back to %d ready pods", ready))
	}
}

// RollingRestart restarts every gateway pod the way `kubectl rollout restart`
// and every chart upgrade does, while traffic flows.
type RollingRestart struct {
	Deployment string

	deployment *appsv1.Deployment
	pods       readiness
	before     map[string]bool
	injectedAt time.Time
	done       bool
}

func (r *RollingRestart) Name() string { return "rolling-restart" }
func (r *RollingRestart) Kind() string { return "rollout" }

func (r *RollingRestart) Prepare(ctx context.Context, k *kube.Client) (string, error) {
	d, err := FindGateway(ctx, k, r.Deployment)
	if err != nil {
		return "", err
	}
	r.deployment = d
	r.pods.selector = labels.SelectorFromSet(d.Spec.Selector.MatchLabels).String()
	pods, err := k.Pods(ctx, r.pods.selector)
	if err != nil {
		return "", err
	}
	r.before = map[string]bool{}
	for _, pod := range pods {
		r.before[pod.Name] = true
	}
	strategy := "Recreate"
	if ru := d.Spec.Strategy.RollingUpdate; ru != nil {
		strategy = fmt.Sprintf("maxSurge %s, maxUnavailable %s", ru.MaxSurge.String(), ru.MaxUnavailable.String())
	}
	return fmt.Sprintf("restart all %d pods of %s (%s)", len(pods), d.Name, strategy), nil
}

func (r *RollingRestart) Inject(ctx context.Context, k *kube.Client) error {
	r.injectedAt = time.Now()
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":%q}}}}}`,
		r.injectedAt.UTC().Format(time.RFC3339))
	_, err := k.Set.AppsV1().Deployments(k.Namespace).Patch(ctx, r.deployment.Name, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("restart deployment %s: %w", r.deployment.Name, err)
	}
	return nil
}

func (r *RollingRestart) Watch(ctx context.Context, k *kube.Client, note func(kind, text string), fact func(key, value string)) {
	ready, total := r.pods.watch(ctx, k, note)
	fact("deployment", r.deployment.Name)
	fact("pods", fmt.Sprint(len(r.before)))
	if r.injectedAt.IsZero() || r.done || ready < 0 {
		return
	}
	for name := range r.pods.last {
		if r.before[name] {
			return
		}
	}
	if ready == total && ready >= len(r.before) {
		r.done = true
		fact("rollout_s", fmt.Sprintf("%.1f", time.Since(r.injectedAt).Seconds()))
		note("pod", fmt.Sprintf("rollout complete: %d new pods ready", ready))
	}
}
