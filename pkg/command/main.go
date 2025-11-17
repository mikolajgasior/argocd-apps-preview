package command

import (
	"bufio"
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
	cmd *exec.Cmd
	stdout *os.File
	stderr *os.File
	waitGroup *sync.WaitGroup
}

func NewCommand(name string, args ...string) (*Command, error) {
	cmd := exec.Command(name, args...)

	stdout, err := os.CreateTemp("", StdoutTmpFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingStdoutTmpFile, err)
	}

	stderr, err := os.CreateTemp("", StderrTmpFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingStderrTmpFile, err)
	}

	command := &Command{
		cmd: cmd,
		stdout: stdout,
		stderr: stderr,
	}

	return command, nil
}

func (c *Command) Run() error {
	fmt.Fprintf(os.Stdout, "🍓 Running command: %s\n", strings.Join(c.cmd.Args, " "))

	err := c.createWaitGroup()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreatingWaitGroup, err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("%w: %w", ErrStartingCommand, err)
	}

	c.waitGroup.Wait()

	if err := c.cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%w: %s", ErrCommandReturnsNonZeroExitCode, exiterr)
		} else {
			return fmt.Errorf("%w: %w", ErrWaitingForCommand, err)
		}
	}

	return nil
}

func (c *Command) SetEnv(env *map[string]string) {
	if env != nil && len(*env) > 0 {
		for k, v := range *env {
			c.cmd.Env = append(c.cmd.Environ(), fmt.Sprintf("%s=%s", k, v))
		}
	}
}

func (c *Command) createWaitGroup() error {
	c.waitGroup = &sync.WaitGroup{}
	c.waitGroup.Add(2)

	cmdStdoutReader, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPipingStdout, err)
	}

	cmdStderrReader, err := c.cmd.StderrPipe()
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
