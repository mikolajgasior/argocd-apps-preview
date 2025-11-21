package argocd

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/keenbytes/argocd-apps-preview/pkg/command"
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
	nodePort int
}

func (a *ArgoCD) Install(ctx context.Context) error {
	fmt.Fprintf(os.Stdout, "🍓 Creating namespace %s...\n", a.namespace)

	err := a.kubeClient.CreateNamespace(ctx, a.namespace)
	if err != nil {
		return fmt.Errorf("creating namespace: %w", err)
	}

	installYaml, err := files.DownloadFile(GetArgoCDManifestURL(a.version))
	if err != nil {
		return fmt.Errorf("downloading argocd manifest failed: %w", err)
	}
	defer os.Remove(installYaml)

	installYaml, err = UpdateManifest(installYaml, a.namespace, a.nodePort)
	if err != nil {
		return fmt.Errorf("updating argocd manifest: %w", err)
	}

	err = a.kubeClient.ApplyFile(ctx, installYaml, a.namespace)
	if err != nil {
		return fmt.Errorf("applying argocd manifest: %w", err)
	}

	err = a.kubeClient.WaitForDeployment(ctx, "argocd-server", a.namespace, 300)
	if err != nil {
		return fmt.Errorf("waiting for argocd: %w", err)
	}

	return nil
}

func (a *ArgoCD) Login(ctx context.Context) error {
	fmt.Fprintf(os.Stdout, "🍓 Logging in to ArgoCD...\n")

	cmd, err := command.NewCommand("kubectl", "get", "secret", "-n", a.namespace, "argocd-initial-admin-secret", "-o", `jsonpath={.data.password}`)
	if err != nil {
		return fmt.Errorf("creating command for getting kind clusters")
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command getting kind cluster failed: %w", err)
	}

	password, err := os.ReadFile(cmd.Stdout().Name())
	if err != nil {
		return fmt.Errorf("getting argocd password output file: %w", err)
	}

	decodedPassword, err := base64.StdEncoding.DecodeString(string(password))
	if err != nil {
		log.Fatalf("base64 decode failed: %v", err)
	}

	cmd, err = command.NewCommand("argocd", "login", fmt.Sprintf("localhost:%d", a.nodePort), "--insecure", "--username", "admin", "--password", string(decodedPassword), "--grpc-web")
	if err != nil {
		return fmt.Errorf("creating command for logging to argocd")
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command for logging to argocd failed: %w", err)
	}

	return nil
}

func (a *ArgoCD) Logout(ctx context.Context) error {
	fmt.Fprintf(os.Stdout, "🍓 Logging out from ArgoCD...\n")

	cmd, err := command.NewCommand("argocd", "logout")
	if err != nil {
		return fmt.Errorf("creating command for logging out of argocd")
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command for logging out of argocd failed: %w", err)
	}

	return nil
}

func (a *ArgoCD) PortForward(ctx context.Context, port int) error {
	err := a.kubeClient.PortForward(ctx, "svc/argocd-server", a.namespace, 443, port)
	if err != nil {
		return fmt.Errorf("port forward for argocd")
	}
	return nil
}

func (a *ArgoCD) GenerateAppsFromAppSets(ctx context.Context, path string) ([]string, error) {
	cmd, err := command.NewCommand("argocd", "appset", "generate", path, "-o", "yaml")
	if err != nil {
		return []string{}, fmt.Errorf("creating command for generating apps")
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return []string{}, fmt.Errorf("command for generating apps failed: %w", err)
	}

	fixedPath, err := FixAppsetGenerateList(cmd.Stdout().Name())
	if err != nil {
		return []string{}, fmt.Errorf("fixing appset generate list: %w", err)
	}

	apps, _, err := kube.ExtractAppsFromYAML(fixedPath)
	if err != nil {
		return []string{}, fmt.Errorf("extracting apps from yaml: %w", err)
	}

	return apps, nil
}

func NewArgoCD(kubeClient *kube.Kube, namespace string, version string, nodePort int) *ArgoCD {
	argocd := &ArgoCD{
		namespace: namespace,
		kubeClient: kubeClient,
		version: version,
		nodePort: nodePort,
	}

	return argocd
}

func GetArgoCDManifestURL(version string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/argoproj/argo-cd/refs/tags/%s/manifests/install.yaml", version)
}

func UpdateManifest(path string, namespace string, nodePort int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)

	var outBuf bytes.Buffer
	encoder := yaml.NewEncoder(&outBuf)
	encoder.SetIndent(2)

	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("yaml decode error: %w", err)
		}

		u := &unstructured.Unstructured{Object: raw}

		if err := patchCmdParamsCm(u); err != nil {
			return "", fmt.Errorf("patching cmd-params-cm resource error: %w", err)
		}

		if err := patchClusterRoleBinding(u, namespace); err != nil {
			return "", fmt.Errorf("patching ClusterRoleBinding resource error: %w", err)
		}

		if err := patchServerService(u, nodePort); err != nil {
			return "", fmt.Errorf("patching server service resource error: %w", err)
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

func FixAppsetGenerateList(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading file %s: %w", path, err)
	}

	var list []interface{}
	err = yaml.Unmarshal(contents, &list)
	if err != nil {
		return path, nil
	}

	dstPath := path + "__fixed.yaml"
	out, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("cannot create %s: %w", dstPath, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("cannot close %s: %w", dstPath, cerr)
		}
	}()

	for i, elem := range list {
		y, err := yaml.Marshal(elem)
		if err != nil {
			return "", fmt.Errorf("cannot marshal element %d: %w", i, err)
		}

		if i > 0 {
			if _, err := out.Write([]byte("---\n")); err != nil {
				return "", fmt.Errorf("cannot write separator: %w", err)
			}
		}

		// Write the element itself.  Ensure it ends with a newline for
		// readability (yaml.Marshal already adds one, but we guard anyway).
		if !bytes.HasSuffix(y, []byte("\n")) {
			y = append(y, '\n')
		}
		if _, err := out.Write(y); err != nil {
			return "", fmt.Errorf("cannot write element %d: %w", i, err)
		}
	}

	return dstPath, nil
}

func patchCmdParamsCm(obj *unstructured.Unstructured) error {
	if obj.GetName() != "argocd-cmd-params-cm" {
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

func patchServerService(u *unstructured.Unstructured, nodePort int) error {
	if u.GetKind() != "Service" || u.GetName() != "argocd-server" {
		return nil
	}

	if err := unstructured.SetNestedField(u.Object, "NodePort", "spec", "type"); err != nil {
		return fmt.Errorf("setting spec.type: %w", err)
	}

	buildPort := func(name string, port, targetPort, nodePort int) map[string]interface{} {
		return map[string]interface{}{
			"name":        name,
			"port":        int64(port),
			"targetPort":  int64(targetPort),
			"nodePort":    int64(nodePort),
		}
	}

	httpsPortSpec := buildPort("https", 443, 8080, nodePort)

	newPorts := []interface{}{httpsPortSpec}
	if err := unstructured.SetNestedSlice(u.Object, newPorts, "spec", "ports"); err != nil {
		return fmt.Errorf("seting spec.ports: %w", err)
	}

	return nil
}
