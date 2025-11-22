package kube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/keenbytes/argocd-apps-preview/pkg/command"
	yamlStd "gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlK8s "sigs.k8s.io/yaml"
)

type Kube struct {
	context string
}

func (k *Kube) CreateNamespace(ctx context.Context, namespace string) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "create", "namespace", namespace)
	if err != nil {
		return fmt.Errorf("creating create namespace command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("command create namespace failed: %w", err)
	}

	return nil
}

func (k *Kube) ApplyFile(ctx context.Context, path string, namespace string) error {
	var cmd *command.Command
	var err error

	if namespace != "" {
		cmd, err = command.NewCommand("kubectl", "--context", k.context, "apply", "-f", path, "--namespace", namespace)
	} else {
		cmd, err = command.NewCommand("kubectl", "--context", k.context, "apply", "-f", path)
	}
	if err != nil {
		return fmt.Errorf("creating argocd manifest command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("running argocd manifest failed: %w", err)
	}

	return nil
}

func (k *Kube) WaitForDeployment(ctx context.Context, deployment string, namespace string, timeout int) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "rollout", "status", fmt.Sprintf("deployment/%s", deployment), "-n", namespace, fmt.Sprintf("--timeout=%ds", timeout))
	if err != nil {
		return fmt.Errorf("creating rollout status command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("running rollout status failed: %w", err)
	}

	return nil
}

func (k *Kube) PortForward(ctx context.Context, name string, namespace string, sourcePort int, destinationPort int) error {
	cmd, err := command.NewCommand("kubectl", "--context", k.context, "port-forward", name, "-n", namespace, fmt.Sprintf("%d:%d", destinationPort, sourcePort))
	if err != nil {
		return fmt.Errorf("creating port-forward command: %w", err)
	}

	err = cmd.Run(ctx, nil)
	if err != nil {
		return fmt.Errorf("running port-forward failed: %w", err)
	}

	return nil
}

func NewKube(kubeContext string) *Kube {
	kube := &Kube{
		context: kubeContext,
	}
	return kube
}

func RemoveNestedField(u *unstructured.Unstructured, fields ...string) bool {
	_, ok, _ := unstructured.NestedFieldNoCopy(u.Object, fields...)
	if ok {
		unstructured.RemoveNestedField(u.Object, fields...)
	}
	return ok
}

func AppendUniqueString(slice []interface{}, val string) []interface{} {
	for _, v := range slice {
		if s, ok := v.(string); ok && s == val {
			return slice
		}
	}
	return append(slice, val)
}

func ExtractAppsFromYAML(path string) ([]string, []string, []string, error) {
f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	tmpDir, err := os.MkdirTemp("", "apps-*")
	if err != nil {
		return []string{}, []string{}, []string{}, fmt.Errorf("creating temp dir for extracted apps from manifests: %w", err)
	}

	apps := []string{}
	appSets := []string{}
	appProjects := []string{}

	decoder := yamlStd.NewDecoder(f)

	var outBuf bytes.Buffer
	encoder := yamlStd.NewEncoder(&outBuf)
	encoder.SetIndent(2)

	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return []string{}, []string{}, []string{}, fmt.Errorf("yaml decode error: %w", err)
		}

		u := &unstructured.Unstructured{Object: raw}
		kind := u.GetKind()
		if kind != "Application" && kind != "ApplicationSet" && kind != "AppProject" {
			continue
		}

		appName := u.GetName()
		appNamespace := u.GetNamespace()
		
		if kind == "Application" {
			_ = RemoveNestedField(u, "spec", "syncPolicy", "automated")

			const optKey = "CreateNamespace=true"
			yamlPath := []string{"spec", "syncPolicy", "syncOptions"}

			existing, _, err := unstructured.NestedSlice(u.Object, yamlPath...)
			if err != nil {
				return []string{}, []string{}, []string{}, fmt.Errorf("reading %v: %v", path, err)
			}

			newSlice := AppendUniqueString(existing, optKey)

			if err := unstructured.SetNestedSlice(u.Object, newSlice, yamlPath...); err != nil {
				return []string{}, []string{}, []string{}, fmt.Errorf("setting %v failed: %v", path, err)
			}
		}

		outFile := fmt.Sprintf("%s/%s___%s___%s.yaml", tmpDir, kind, appNamespace, appName)
		if err := WriteObjectAsYAML(u, outFile); err != nil {
			return []string{}, []string{}, []string{}, fmt.Errorf("cannot write %s: %w", outFile, err)
		}

		switch kind {
		case "Application":
			apps = append(apps, outFile)
		case "ApplicationSet":
			appSets = append(appSets, outFile)
		case "AppProject":
			appProjects = append(appProjects, outFile)
		default:
		}
	}

	_ = encoder.Close()

	return apps, appSets, appProjects, nil
}

func WriteObjectAsYAML(obj *unstructured.Unstructured, outPath string) error {
	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("cannot marshal %s to JSON: %w", outPath, err)
	}

	yamlBytes, err := yamlK8s.JSONToYAML(jsonBytes)
	if err != nil {
		return fmt.Errorf("cannot convert JSON→YAML for %s: %w", outPath, err)
	}

	if len(yamlBytes) == 0 || yamlBytes[len(yamlBytes)-1] != '\n' {
		yamlBytes = append(yamlBytes, '\n')
	}

	if err := os.WriteFile(outPath, yamlBytes, 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", outPath, err)
	}
	return nil
}
