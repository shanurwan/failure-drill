// Package kube is the small part of the Kubernetes API the drills need.
package kube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Client talks to one namespace of one cluster.
type Client struct {
	Set       kubernetes.Interface
	Namespace string
	config    *rest.Config
}

// New loads the kubeconfig the way kubectl does: $KUBECONFIG, then
// ~/.kube/config, then the in-cluster service account. An empty namespace
// means the one the context names.
func New(kubeContext, namespace string) (*Client, error) {
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	)
	config, err := loader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	if namespace == "" {
		if namespace, _, err = loader.Namespace(); err != nil {
			return nil, fmt.Errorf("read the context's namespace: %w", err)
		}
	}
	set, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes client: %w", err)
	}
	return &Client{Set: set, Namespace: namespace, config: config}, nil
}

// Pods lists the pods a label selector matches.
func (c *Client) Pods(ctx context.Context, selector string) ([]corev1.Pod, error) {
	list, err := c.Set.CoreV1().Pods(c.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list pods %q: %w", selector, err)
	}
	return list.Items, nil
}

// Ready reports whether the pod is running and passes its readiness probe.
func Ready(pod corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// DeletePod removes a pod. A zero grace period kills it without warning, the
// way a crashed node would; nil uses the pod's own grace period, the way a
// drain does.
func (c *Client) DeletePod(ctx context.Context, name string, gracePeriod *int64) error {
	err := c.Set.CoreV1().Pods(c.Namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: gracePeriod})
	if err != nil {
		return fmt.Errorf("delete pod %s: %w", name, err)
	}
	return nil
}

// Exec runs a command in a pod and returns its standard output. It gives up
// after timeout, which is how the drills notice a process that has hung.
func (c *Client) Exec(ctx context.Context, pod, container string, timeout time.Duration, command ...string) (string, error) {
	var stdout bytes.Buffer
	if _, err := c.ExecIO(ctx, pod, container, timeout, nil, &stdout, command...); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// ExecIO runs a command in a pod with its input and output attached, for
// commands that move data: a database dump out, a restore in. It returns what
// the command wrote to standard error, which is where those tools report.
func (c *Client) ExecIO(ctx context.Context, pod, container string, timeout time.Duration, stdin io.Reader, stdout io.Writer, command ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if stdout == nil {
		stdout = io.Discard
	}

	req := c.Set.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(c.Namespace).Name(pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("exec in %s: %w", pod, err)
	}
	var stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: stdin, Stdout: stdout, Stderr: &stderr})
	if err != nil {
		return stderr.String(), fmt.Errorf("exec %q in %s: %w %s", strings.Join(command, " "), pod, err, strings.TrimSpace(stderr.String()))
	}
	return stderr.String(), nil
}
