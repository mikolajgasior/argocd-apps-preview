package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	ArgoCDNodePort = 30443
)

const (
	ExitKindNotFound = 101
	ExitArgoCDNotFound = 102
	ExitKubectlNotFound = 103
	ExitCreatingClusterFailed = 201
	ExitDeletingClusterFailed = 202
	ExitArgoCDInstallationFailed = 301
	ExitArgoCDPortForwardFailed = 302
	ExitArgoCDLoggingFailed = 303
	ExitApplyingSecretsFailed = 304
	ExitApplyingManifestsFailed = 305
)

func main() {
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

	acd := argocd.NewArgoCD(kubeClient, ArgoCDNamespace, ArgoCDVersion, ArgoCDNodePort)

	ctxArgoCD, cancelArgoCD := context.WithTimeout(context.Background(), 360 * time.Second)
	defer cancelArgoCD()

	err = acd.Install(ctxArgoCD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error installing argocd: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitArgoCDInstallationFailed)
	}

	time.Sleep(4 * time.Second)

	err = acd.Login(ctxArgoCD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error logging into argocd: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitArgoCDLoggingFailed)
	}

	err = applyManifestsFromDir(ctxArgoCD, kubeClient, DirSecrets, ArgoCDNamespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error applying files from secrets directory: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitApplyingSecretsFailed)
	}

	err = applyManifestsFromDir(ctxArgoCD, kubeClient, DirManifests, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error applying files from manifests directory: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitApplyingManifestsFailed)
	}

	cluster.Delete()
	os.Exit(0)
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

func applyManifestsFromDir(ctx context.Context, kubeClient *kube.Kube, dir string, namespace string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("getting stat for directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory", dir)
	}

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".yaml" || ext == ".yml" {
			err2 := kubeClient.ApplyFile(ctx, path, namespace)
			if err2 != nil {
				return fmt.Errorf("applying manifest from %s: %w", dir, err2)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking directory %s: %v", dir, err)
	}

	return nil	
}
