package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	ExitKindNotFound = 101
	ExitArgoCDNotFound = 102
	ExitKubectlNotFound = 103
	ExitCreatingClusterFailed = 201
)

const (
	KindName = "argocd-app-prev"
	KindImage = "kindest/node:v1.33.4"
)

func main() {
	// 05. install argocd - before that modify the ClusterRoleBinding -> kubectl
		// + wait until it's deployed
	  // + port-forward 8080:443
	// 06. login to argocd cd -> get passwd from kube then call argocd login
	// 07. apply secrets
	// 08. apply initial application(s)
	// 09. recursively scan and apply applications
	// 10. dump applications from argocd
	checkPrerequisites()
	createCluster(KindName)
	deleteCluster(KindName)

	os.Exit(0)
}

func checkPrerequisites() {
	fmt.Fprintf(os.Stdout, "🍓 Checking prerequisites...\n")

	_, err := exec.LookPath("kind")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kind not found")
		os.Exit(ExitKindNotFound)
	}

//	_, err = exec.LookPath("argocd-cli")
//	if err != nil {
//		slog.Error("argocd-cli not found")
//		os.Exit(ExitArgoCDNotFound)
//	}
//
//	_, err = exec.LookPath("kubectl")
//	if err != nil {
//		slog.Error("kubectl not found")
//		os.Exit(ExitKubectlNotFound)
//	}
}

func createCluster(kindName string) error {
	fmt.Fprintf(os.Stdout, "🍓 Creating cluster...\n")

	env := map[string]string{}

	_, _, err := runCommand(&env, "kind", "create", "cluster", "-n", kindName, "--image", KindImage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error running command that creates cluster: %s\n", err.Error())
		return fmt.Errorf("error running cmd that creates kind cluster: %w", err)
	}

	return nil
}

func deleteCluster(kindName string) error {
	fmt.Fprintf(os.Stdout, "🍓 Deleting cluster...\n")

	env := map[string]string{}

	_, _, err := runCommand(&env, "kind", "delete", "cluster", "-n", kindName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error running command that deletes cluster: %s\n", err.Error())
		return fmt.Errorf("error running cmd that deletes kind cluster: %w", err)
	}

	return nil
}

func createCmd(env *map[string]string, name string, args ...string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.Command(name, args...)

	if env != nil && len(*env)>0 {
		for k, v := range *env {
			cmd.Env = append(cmd.Environ(), fmt.Sprintf("%s=%s", k, v))
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error piping stdout: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error piping stderr: %w", err)
	}

	return cmd, stdout, stderr, nil
}

func createWaitGroup(stdout io.ReadCloser, fOut *os.File, stderr io.ReadCloser, fErr *os.File, quiet bool) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		scanner := bufio.NewScanner(stdout)
		var writer io.Writer
		writer = io.MultiWriter(fOut, os.Stdout)
		for scanner.Scan() {
			fmt.Fprintln(writer, scanner.Text())
		}
		wg.Done()
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		var writer io.Writer
		writer = io.MultiWriter(fErr, os.Stderr)
		for scanner.Scan() {
			fmt.Fprintln(writer, scanner.Text())
		}
		wg.Done()
	}()
	return wg
}

func initCommandOutputs() (*os.File, *os.File, error) {
	fOut, err := os.CreateTemp("", "argocd-apps-prev-stdout.*.txt")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tmp file for stdout: %w", err)
	}

	fErr, err := os.CreateTemp("", "argocd-apps-prev-stderr.*.txt")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tmp file for stderr: %w", err)
	}

	return fOut, fErr, nil
}

func runCommand(env *map[string]string, name string, args ...string) (string, string, error) {
	fOut, fErr, err := initCommandOutputs()
	if err != nil {
		return "", "", fmt.Errorf("error initializing step outputs: %w", err)
	}

	nameAndArgs := make([]string, len(args)+1)
	nameAndArgs[0] = name
	nameAndArgs = append(nameAndArgs, args...)
	fmt.Fprintf(os.Stdout, "🍓 Running command: %s\n", strings.Join(nameAndArgs, " "))

	cmd, stdout, stderr, err := createCmd(env, name, args...)
	if err != nil {
		return "", "", fmt.Errorf("error creating command: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("error starting command: %w", err)
	}

	// create wait group that attaches stdout and stderr to files
	wg := createWaitGroup(stdout, fOut, stderr, fErr, false)
	wg.Wait()

	// wait for the command to finish
	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			return fOut.Name(), fErr.Name(), fmt.Errorf("command returns exit code %s", exiterr)
		} else {
			return "", "", fmt.Errorf("error waiting for the command: %w", err)
		}
	}

	return fOut.Name(), fErr.Name(), nil
}
