package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/keenbytes/argocd-apps-preview/pkg/argocd"
	"github.com/keenbytes/argocd-apps-preview/pkg/kind"
	"github.com/keenbytes/argocd-apps-preview/pkg/kube"
)

const (
	KindName = "argocd-app-prev"
	KindImage = "kindest/node:v1.33.4"
	ArgoCDNamespace = "deployments"
	ArgoCDVersion = "v2.14.11"
	DirManifests = "manifests"
	DirSecrets = "secrets"
	ArgoCDPortForward = 30080
)

const (
	ExitKindNotFound = 101
	ExitArgoCDNotFound = 102
	ExitKubectlNotFound = 103
	ExitCreatingClusterFailed = 201
	ExitDeletingClusterFailed = 202
	ExitArgoCDInstallationFailed = 301
	ExitArgoCDPortForwardFailed = 302
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

	cluster := kind.NewKind(KindName, KindImage)
	_ = cluster.Delete()

	ctxCluster, cancelCluster := context.WithTimeout(context.Background(), 120 * time.Second)
	defer cancelCluster()

	err := cluster.Create(ctxCluster)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error creating kind cluster: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitCreatingClusterFailed)
	}

	kubeClient := kube.NewKube(getKubeContext())

	acd := argocd.NewArgoCD(kubeClient, ArgoCDNamespace, ArgoCDVersion)

	ctxArgoCD, cancelArgoCD := context.WithTimeout(context.Background(), 360 * time.Second)
	defer cancelArgoCD()

	err = acd.Install(ctxArgoCD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error installing argocd: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitArgoCDInstallationFailed)
	}

	ctxPortForward, cancelPortForward := context.WithTimeout(context.Background(), 3600 * time.Second)
	defer cancelPortForward()

	waitGroup := &sync.WaitGroup{}
	waitGroup.Add(2)

	exitError := 0

	go func() {
		err = acd.PortForward(ctxPortForward, ArgoCDPortForward)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error forwarding port to argocd: %s", err.Error())
			exitError = ExitArgoCDPortForwardFailed
		}
		waitGroup.Done()
	}()

	go func() {
		time.Sleep(60 * time.Second)
		cancelPortForward()
		waitGroup.Done()
	}()

	waitGroup.Wait()

	cluster.Delete()
	os.Exit(exitError)
}

func checkPrerequisites() {
	fmt.Fprintf(os.Stdout, "🍓 Checking prerequisites...\n")

	_, err := exec.LookPath("kind")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ kind not found")
		os.Exit(ExitKindNotFound)
	}

	_, err = exec.LookPath("argocd")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ argocd-cli not found")
		os.Exit(ExitArgoCDNotFound)
	}

	_, err = exec.LookPath("kubectl")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ kubectl not found")
		os.Exit(ExitKubectlNotFound)
	}
}

func getKubeContext() string {
	return fmt.Sprintf("kind-%s", KindName)
}
