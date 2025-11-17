package argocd

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/keenbytes/argocd-apps-preview/pkg/files"
	"github.com/keenbytes/argocd-apps-preview/pkg/kube"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	installYaml, err := files.DownloadFile(GetArgoCDManifestURL(a.version))
	if err != nil {
		return fmt.Errorf("downloading argocd manifest failed: %w", err)
	}
	defer os.Remove(installYaml)
fmt.Fprint(os.Stdout, installYaml)
	installYaml, err = UpdateManifestToAllowAllNamespaces(installYaml, a.namespace)
	if err != nil {
		return fmt.Errorf("updating argocd manifest to allow all namespaces: w", err)
	}
fmt.Fprint(os.Stdout, installYaml)
	err = a.kubeClient.ApplyFile(installYaml, a.namespace)
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

func UpdateManifestToAllowAllNamespaces(path string, namespace string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)

	var outBuf bytes.Buffer
	encoder := yaml.NewEncoder(&outBuf)
	encoder.SetIndent(2)

	const target = "argocd-cmd-params-cm"

	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("yaml decode error: %w", err)
		}

		u := &unstructured.Unstructured{Object: raw}

		if err := patchCmdParamsCm(u, target); err != nil {
			return "", fmt.Errorf("patching cmd-params-cm resource error: %w", err)
		}

		if err := patchClusterRoleBinding(u, namespace); err != nil {
			return "", fmt.Errorf("patching ClusterRoleBinding resource error: %w", err)
		}

		if err := encoder.Encode(u.Object); err != nil {
			return "", fmt.Errorf("yaml encode error: %w", err)
		}
	}

	_ = encoder.Close()

	outPath := path + ".upd.yaml"

	if err := os.WriteFile(outPath, outBuf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("updating argocd manifest: %w", err)
	}

	return outPath, nil
}

func patchCmdParamsCm(obj *unstructured.Unstructured, targetName string) error {
	if obj.GetName() != targetName {
		return nil
	}

	data, _, err := unstructured.NestedStringMap(obj.Object, "data")
	if err != nil {
		return fmt.Errorf("reading .data: %w", err)
	}
	if data == nil {
		data = map[string]string{}
	}

	data["application.namespaces"] = "*"

	if err := unstructured.SetNestedStringMap(obj.Object, data, "data"); err != nil {
		return fmt.Errorf("writing .data: %w", err)
	}
	return nil
}

func patchClusterRoleBinding(u *unstructured.Unstructured, namespace string) error {
	if u.GetKind() != "ClusterRoleBinding" || (u.GetName() != "argocd-application-controller" && u.GetName() != "argocd-applicationset-controller" && u.GetName() != "argocd-server") {
		return nil
	}

	subjects, found, err := unstructured.NestedSlice(u.Object, "subjects")
	if err != nil {
		return fmt.Errorf("reading .subjects: %w", err)
	}
	if !found || len(subjects) == 0 {
		subjects = []interface{}{map[string]interface{}{}}
	}

	first, ok := subjects[0].(map[string]interface{})
	if !ok {
		first = map[string]interface{}{}
	}

	first["namespace"] = namespace

	subjects[0] = first

	if err := unstructured.SetNestedSlice(u.Object, subjects, "subjects"); err != nil {
		return fmt.Errorf("writing .subjects: %w", err)
	}
	return nil
}
