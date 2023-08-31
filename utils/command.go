package utils

import (
	"bytes"
	"context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"strings"
)

type RemoteRequest struct {
	K8sConfig *rest.Config
	ClientSet kubernetes.Interface
}

func (e *RemoteRequest) Exec(ctx context.Context, namespace, podName, containerName string, cmd []string) (string, string, error) {
	req := e.ClientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).SubResource("exec").Param("container", containerName)
	req.VersionedParams(
		&v1.PodExecOptions{
			Command: cmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		},
		scheme.ParameterCodec,
	)
	var stdout, stderr bytes.Buffer
	exec, err := remotecommand.NewSPDYExecutor(e.K8sConfig, "POST", req.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func NewRemoteRequest() (*RemoteRequest, error) {
	k8sCli, err := kubernetes.NewForConfig(config.GetConfigOrDie())
	if err != nil {
		return nil, err
	}
	return &RemoteRequest{
		K8sConfig: config.GetConfigOrDie(),
		ClientSet: k8sCli,
	}, nil
}
