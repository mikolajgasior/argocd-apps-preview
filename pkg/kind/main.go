package kind

import (
	"fmt"
	"os"

	"github.com/keenbytes/argocd-apps-preview/pkg/command"
)

type Kind struct {
	name string
	image string
}

func (k *Kind) Create() error {
	fmt.Fprintf(os.Stdout, "🍓 Creating cluster...\n")

	command, err := command.NewCommand("kind", "create", "cluster", "-n", k.name, "--image", k.image)
	if err != nil {
		return fmt.Errorf("creating command to create kind cluster: %w", err)
	}

	err = command.Run()
	if err != nil {
		return fmt.Errorf("command creating kind cluster failed: %w", err)
	}

	return nil
}

func (k *Kind) Delete() error {
	fmt.Fprintf(os.Stdout, "🍓 Deleting cluster...\n")

	cmd, err := command.NewCommand("kind", "get", "clusters")
	if err != nil {
		return fmt.Errorf("creating command for getting kind clusters")
	}
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("command getting kind cluster failed: %w", err)
	}

	cmd, err = command.NewCommand("kind", "delete", "cluster", "-n", k.name)
	if err != nil {
		return fmt.Errorf("creating command for deleting kind cluster: %w", err)
	}
	err = cmd.Run()
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
