package preflight

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/shanurwan/failure-drill/internal/kube"
)

const ns = "tyk"

var gatewayLabels = map[string]string{"app": "gateway"}

func gateway(replicas int32, edit func(*appsv1.Deployment)) *appsv1.Deployment {
	one := intstr.FromInt32(1)
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: gatewayLabels},
			Strategy: appsv1.DeploymentStrategy{RollingUpdate: &appsv1.RollingUpdateDeployment{MaxUnavailable: &one}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: gatewayLabels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "gateway",
					Image: "docker.tyk.io/tyk-gateway/tyk-gateway:v5.13.5",
					Env: []corev1.EnvVar{
						{Name: "TYK_GW_STORAGE_ADDRS", Value: "redis:6379"},
					},
				}}},
			},
		},
	}
	if edit != nil {
		edit(d)
	}
	return d
}

func pod(name, node string, labels map[string]string, persistent bool) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Spec:       corev1.PodSpec{NodeName: node},
	}
	if persistent {
		p.Spec.Volumes = []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data-" + name},
		}}}
	}
	return p
}

func run(t *testing.T, d *appsv1.Deployment, objects ...runtime.Object) map[string]Finding {
	t.Helper()
	k := &kube.Client{Set: fake.NewClientset(objects...), Namespace: ns}
	report, err := Run(context.Background(), k, d, "app=redis")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Finding{}
	for _, f := range report.Findings {
		byID[f.ID] = f
	}
	return byID
}

func want(t *testing.T, findings map[string]Finding, id string, level Level) {
	t.Helper()
	f, ok := findings[id]
	if !ok {
		t.Errorf("no %q finding", id)
		return
	}
	if f.Level != level {
		t.Errorf("%q is %s (%s), want %s", id, f.Level, f.What, level)
	}
}

// What a chart's defaults give you: one gateway, one Redis with no volume.
func TestBareInstall(t *testing.T) {
	redis := map[string]string{"app": "redis"}
	findings := run(t, gateway(1, nil),
		pod("gateway-a", "node-1", gatewayLabels, false),
		pod("redis-0", "node-1", redis, false),
	)

	want(t, findings, "replicas", Warn)
	want(t, findings, "pdb", Warn)
	want(t, findings, "prestop", Warn)
	want(t, findings, "readiness", Warn)
	want(t, findings, "redis-mode", Warn)
	want(t, findings, "redis-pods", Warn)
	want(t, findings, "redis-storage", Warn)
	if _, ok := findings["spread"]; ok {
		t.Error("one replica cannot be spread: no spread finding expected")
	}
}

func TestPreparedInstall(t *testing.T) {
	redis := map[string]string{"app": "redis"}
	zero := intstr.FromInt32(0)
	d := gateway(3, func(d *appsv1.Deployment) {
		d.Spec.Strategy.RollingUpdate.MaxUnavailable = &zero
		spec := &d.Spec.Template.Spec
		spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{TopologyKey: "kubernetes.io/hostname"}}
		c := &spec.Containers[0]
		c.Lifecycle = &corev1.Lifecycle{PreStop: &corev1.LifecycleHandler{Sleep: &corev1.SleepAction{Seconds: 10}}}
		c.ReadinessProbe = &corev1.Probe{
			ProbeHandler:     corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/hello"}},
			PeriodSeconds:    2,
			FailureThreshold: 3,
		}
		c.Env = []corev1.EnvVar{
			{Name: "TYK_GW_STORAGE_ADDRS", Value: "sentinel:26379"},
			{Name: "TYK_GW_STORAGE_MASTERNAME", Value: "mymaster"},
		}
	})
	budget := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: ns},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: gatewayLabels}},
	}
	findings := run(t, d, budget,
		pod("gateway-a", "node-1", gatewayLabels, false),
		pod("gateway-b", "node-2", gatewayLabels, false),
		pod("gateway-c", "node-3", gatewayLabels, false),
		pod("redis-0", "node-1", redis, true),
		pod("redis-1", "node-2", redis, true),
		pod("redis-2", "node-3", redis, true),
	)

	for _, id := range []string{"replicas", "spread", "pdb", "prestop", "readiness", "redis-mode", "redis-pods", "redis-storage"} {
		want(t, findings, id, OK)
	}
	if f, ok := findings["rollout"]; ok {
		t.Errorf("maxUnavailable 0 needs no rollout finding, got %q", f.What)
	}
}

// Two replicas that happen to share a node are no better than one when the
// node goes, whatever the replica count says.
func TestReplicasOnOneNode(t *testing.T) {
	findings := run(t, gateway(2, nil),
		pod("gateway-a", "node-1", gatewayLabels, false),
		pod("gateway-b", "node-1", gatewayLabels, false),
	)
	want(t, findings, "replicas", OK)
	want(t, findings, "spread", Warn)
	want(t, findings, "rollout", Warn)
}

// A budget for some other workload does not cover the gateway.
func TestBudgetForAnotherWorkload(t *testing.T) {
	other := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: ns},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "other"}}},
	}
	findings := run(t, gateway(2, nil), other)
	want(t, findings, "pdb", Warn)
}
