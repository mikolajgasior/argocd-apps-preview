package kube

import (
	"fmt"

	"github.com/keenbytes/argocd-apps-preview/pkg/command"
)

type Kube struct {
	context string
}

func (k *Kube) CreateNamespace(namespace string) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "create", "namespace", namespace)
	if err != nil {
		return fmt.Errorf("creating create namespace command: %w", err)
	}

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("command create namespace failed: %w", err)
	}

	return nil
}

func (k *Kube) ApplyFile(path string, namespace string) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "apply", "-f", path, "-n", namespace)
	if err != nil {
		return fmt.Errorf("creating argocd manifest command: %w", err)
	}

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("running argocd manifest failed: %w", err)
	}

	return nil
}

func (k *Kube) WaitForDeployment(deployment string, namespace string, timeout int) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "rollout", "status", fmt.Sprintf("deployment/%s", deployment), "-n", namespace, fmt.Sprintf("--timeout=%ds", timeout))
	if err != nil {
		return fmt.Errorf("creating rollout status command: %w", err)
	}

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("running rollout status failed: %w", err)
	}

	return nil
}

func NewKube(kubeContext string) *Kube {
	kube := &Kube{
		context: kubeContext,
	}
	return kube
}
