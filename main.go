package main

import (
	"flag"
	"os"
	"os/exec"
)

const (
	ExitKindNotFound = 101
	ExitArgoCDNotFound = 102
	ExitKubectlNotFound = 103
)

func main() {
	// 04. start kind cluster
	// 05. install argocd - before that modify the ClusterRoleBinding -> kubectl
		// + wait until it's deployed
	  // + port-forward 8080:443
	// 06. login to argocd cd -> get passwd from kube then call argocd login
	// 07. apply secrets
	// 08. apply initial application(s)
	// 09. recursively scan and apply applications
	// 10. dump applications from argocd
	kindName, argocdNs, argocdVer, manifestsPath, secretsPath := parseFlags()
	checkPrerequisites()

	os.Exit(0)
}

func parseFlags() (string, string, string, string, string) {
	kindName := flag.String("kind-name", "argocd-apps-prev", "Name of the Kind cluster")
	argocdNs := flag.String("argocd-namespace", "deployments", "Namespace where ArgoCD should be installed")
	argocdVer := flag.String("argocd-version", "v2.14.11", "Version of ArgoCD to use")
	manifestsPath := flag.String("manifests-path", "manifests", "Path to Kubernetes manifest to apply")
	secretsPath := flag.String("secrets-path", "secrets", "Path to ArgoCD manifests with secrets")

	return kindName, argocdNs, argocdVer, manifestsPath, secretsPath
}

func checkPrerequisites() {
	_, err := exec.LookPath("kind")
	if err != nil {
		os.Exit(ExitKindNotFound)
	}

	_, err = exec.LookPath("argocd-cli")
	if err != nil {
		os.Exit(ExitArgoCDNotFound)
	}

	_, err := exec.LookPath("kubectl")
	if err != nil {
		os.Exit(ExitKubectlNotFound)
	}
}
