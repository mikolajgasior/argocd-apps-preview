package kind

import (
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/keenbytes/argocd-apps-preview/pkg/command"
	"github.com/keenbytes/argocd-apps-preview/pkg/logmsg"
)

//go:embed cluster.yml
var clusterConfig []byte

type Kind struct {
	name  string
	image string
}

func (k *Kind) Create(ctx context.Context) error {
	logmsg.Info("Creating cluster...")

	config, err := writeEmbeddedYAMLToTemp()
	if err != nil {
		return fmt.Errorf("writing cluster config")
	}

	command, err := command.NewCommand("kind", "create", "cluster", "-n", k.name, "--image", k.image, "--config", config)
	if err != nil {
		return fmt.Errorf("creating command to create kind cluster: %w", err)
	}

	err = command.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command creating kind cluster failed: %w", err)
	}

	return nil
}

func (k *Kind) Delete() error {
	logmsg.Info("Deleting cluster...")

	ctx := context.Background()

	cmd, err := command.NewCommand("kind", "get", "clusters")
	if err != nil {
		return fmt.Errorf("creating command for getting kind clusters")
	}
	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command getting kind cluster failed: %w", err)
	}

	cmd, err = command.NewCommand("kind", "delete", "cluster", "-n", k.name)
	if err != nil {
		return fmt.Errorf("creating command for deleting kind cluster: %w", err)
	}
	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command deleting kind cluster failed: %w", err)
	}

	return nil
}

func NewKind(name string, image string) *Kind {
	kind := &Kind{}
	kind.name = name
	return kind
}

func writeEmbeddedYAMLToTemp() (string, error) {
	tmpDir, err := os.MkdirTemp("", "cluster-yaml-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	tmpPath := filepath.Join(tmpDir, "cluster.yml")

	if err := os.WriteFile(tmpPath, clusterConfig, fs.FileMode(0600)); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("writing yaml to %s: %w", tmpPath, err)
	}

	return tmpPath, nil
}
