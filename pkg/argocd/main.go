package argocd

import (
	"fmt"
	"os"

	"github.com/keenbytes/argocd-apps-preview/pkg/files"
	"github.com/keenbytes/argocd-apps-preview/pkg/kube"
)

type ArgoCD struct {
	kubeClient *kube.Kube
	namespace string
	started bool
	version string
}

func (a *ArgoCD) Install() error {
	fmt.Fprintf(os.Stdout, "🍓 Creating namespace %s...\n", a.namespace)

	err := a.kubeClient.CreateNamespace(a.namespace)
	if err != nil {
		return fmt.Errorf("creating namespace: %w", err)
	}

	manifestFile, err := files.DownloadFile(GetArgoCDManifestURL(a.version))
	if err != nil {
		return fmt.Errorf("downloading argocd manifest failed: %w", err)
	}
	defer os.Remove(manifestFile)

	err = a.kubeClient.ApplyFile(manifestFile, a.namespace)
	if err != nil {
		return fmt.Errorf("applying argocd manifest: %w", err)
	}

	err = a.kubeClient.WaitForDeployment("argocd-server", a.namespace, 300)
	if err != nil {
		return fmt.Errorf("waiting for argocd: %w", err)
	}

	return nil
}

func NewArgoCD(kubeClient *kube.Kube, namespace string, version string) *ArgoCD {
	argocd := &ArgoCD{
		namespace: namespace,
		kubeClient: kubeClient,
		version: version,
	}

	return argocd
}

func GetArgoCDManifestURL(version string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/argoproj/argo-cd/refs/tags/%s/manifests/install.yaml", version)
}
