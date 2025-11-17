package kube

import (
	"context"
	"fmt"

	"github.com/keenbytes/argocd-apps-preview/pkg/command"
)

type Kube struct {
	context string
}

func (k *Kube) CreateNamespace(ctx context.Context, namespace string) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "create", "namespace", namespace)
	if err != nil {
		return fmt.Errorf("creating create namespace command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command create namespace failed: %w", err)
	}

	return nil
}

func (k *Kube) ApplyFile(ctx context.Context, path string, namespace string) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "apply", "-f", path, "-n", namespace)
	if err != nil {
		return fmt.Errorf("creating argocd manifest command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("running argocd manifest failed: %w", err)
	}

	return nil
}

func (k *Kube) WaitForDeployment(ctx context.Context, deployment string, namespace string, timeout int) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "rollout", "status", fmt.Sprintf("deployment/%s", deployment), "-n", namespace, fmt.Sprintf("--timeout=%ds", timeout))
	if err != nil {
		return fmt.Errorf("creating rollout status command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("running rollout status failed: %w", err)
	}

	return nil
}

func (k *Kube) PortForward(ctx context.Context, name string, namespace string, sourcePort int, destinationPort int) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "port-forward", name, "-n", namespace, fmt.Sprintf("%d:%d", destinationPort, sourcePort))
	if err != nil {
		return fmt.Errorf("creating port-forward command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("running port-forward failed: %w", err)
	}

	return nil
}

func NewKube(kubeContext string) *Kube {
	kube := &Kube{
		context: kubeContext,
	}
	return kube
}
