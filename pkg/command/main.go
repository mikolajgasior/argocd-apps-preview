package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	StdoutTmpFile = "argocd-apps-prev-stdout.*.txt"
	StderrTmpFile = "argocd-apps-prev-stderr.*.txt"
)

var (
	ErrCreatingCommand = errors.New("error creating command")
	ErrStartingCommand = errors.New("error starting command")
	ErrCreatingWaitGroup = errors.New("error creating wait group")
	ErrCommandReturnsNonZeroExitCode = errors.New("command returns non-zero exit code")
	ErrWaitingForCommand = errors.New("error waiting for the command")
	ErrPipingStdout = errors.New("error piping stdout")
	ErrPipingStderr = errors.New("error piping stderr")
	ErrCreatingStdoutTmpFile = errors.New("error creating tmp file for stdout")
	ErrCreatingStderrTmpFile = errors.New("error creating tmp file for stderr")
)

type Command struct {
	name string
	args []string
	stdout *os.File
	stderr *os.File
	waitGroup *sync.WaitGroup
}

func (c *Command) Stdout() *os.File {
	return c.stdout
}

func NewCommand(name string, args ...string) (*Command, error) {
	stdout, err := os.CreateTemp("", StdoutTmpFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingStdoutTmpFile, err)
	}

	stderr, err := os.CreateTemp("", StderrTmpFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingStderrTmpFile, err)
	}

	command := &Command{
		name: name,
		args: args,
		stdout: stdout,
		stderr: stderr,
	}

	return command, nil
}

func (c *Command) Run(ctx context.Context, env *map[string]string) error {
	fmt.Fprintf(os.Stdout, "🍓 Running command: %s %s\n", c.name, strings.Join(c.args, " "))

	cmd := exec.CommandContext(ctx, c.name, c.args...)

	err := c.createWaitGroup(cmd)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreatingWaitGroup, err)
	}

	if env != nil && len(*env) > 0 {
		for k, v := range *env {
			cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", k, v))
		}
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %w", ErrStartingCommand, err)
	}

	c.waitGroup.Wait()

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%w: %s", ErrCommandReturnsNonZeroExitCode, exiterr)
		}
		
		if errors.Is(err, context.Canceled) {
			return nil
		}
		
		return fmt.Errorf("%w: %w", ErrWaitingForCommand, err)
	}

	return nil
}

func (c *Command) createWaitGroup(cmd *exec.Cmd) error {
	c.waitGroup = &sync.WaitGroup{}
	c.waitGroup.Add(2)

	cmdStdoutReader, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPipingStdout, err)
	}

	cmdStderrReader, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPipingStderr, err)
	}

	go func() {
		scanner := bufio.NewScanner(cmdStdoutReader)
		var writer io.Writer
		writer = io.MultiWriter(c.stdout, os.Stdout)
		for scanner.Scan() {
			fmt.Fprintln(writer, scanner.Text())
		}
		c.waitGroup.Done()
	}()

	go func() {
		scanner := bufio.NewScanner(cmdStderrReader)
		var writer io.Writer
		writer = io.MultiWriter(c.stderr, os.Stderr)
		for scanner.Scan() {
			fmt.Fprintln(writer, scanner.Text())
		}
		c.waitGroup.Done()
	}()

	return nil
}
