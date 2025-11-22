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
	ArgoCDNamespace = "tools"
	ArgoCDVersion = "v2.14.11"
	DirManifests = "manifests"
	DirSecrets = "secrets"
	ArgoCDNodePort = 30443
	MaxRecursions = 6
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
	ExitRecursivelyApplyingAppsFailed = 306
)

func main() {
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

	err = applyManifests(ctxArgoCD, kubeClient, DirSecrets, ArgoCDNamespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error applying files from secrets directory: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitApplyingSecretsFailed)
	}

	err = applyAppManifestsFromDir(ctxArgoCD, kubeClient, acd, DirManifests)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error applying applications from manifests directory: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitApplyingManifestsFailed)
	}

	ctxRecursiveApply, cancelRecursiveApply := context.WithTimeout(context.Background(), 360 * time.Second)
	defer cancelRecursiveApply()

	numRecursions := 0
	processedApps := map[string]struct{}{}
	err = recursivelyApplyApps(ctxRecursiveApply, kubeClient, acd, &numRecursions, &processedApps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error recursively applying applications: %s", err.Error())
		cluster.Delete()
		os.Exit(ExitRecursivelyApplyingAppsFailed)
	}

	//cluster.Delete()
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

func applyManifests(ctx context.Context, kubeClient *kube.Kube, dir string, namespace string) error {
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

func applyAppManifestsFromDir(ctx context.Context, kubeClient *kube.Kube, acd *argocd.ArgoCD, dir string) error {
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
			_, err2 := extractAndApplyAppsFromManifestsYAML(ctx, path, kubeClient, acd)
			if err2 != nil {
				return fmt.Errorf("extracting and applying apps from manifests yaml: %w", err2)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking directory %s: %v", dir, err)
	}

	return nil	
}

func extractAndApplyAppsFromManifestsYAML(ctx context.Context, path string, kubeClient *kube.Kube, acd *argocd.ArgoCD) (bool, error) {
	apps, appSets, appProjects, err := kube.ExtractAppsFromYAML(path)
	if err != nil {
		return false, fmt.Errorf("extracting apps from yaml: %w", err)
	}

	if len(appProjects) > 0 {
		for _, appProject := range appProjects {
			err2 := kubeClient.ApplyFile(ctx, appProject, acd.Namespace())
			if err2 != nil {
				return false, fmt.Errorf("applying app project manifest from %s: %w", appProject, err2)
			}
		}
	}

	if len(appSets) > 0 {
		for _, appSet := range appSets {
			genApps, err := acd.GenerateAppsFromAppSets(ctx, appSet)
			if err != nil {
				base := filepath.Base(appSet)
				
				return false, fmt.Errorf("generating apps from appset %s: %w", base, err)
			}

			for _, genApp := range genApps {
				apps = append(apps, genApp)
			}
		}
	}

	if len(apps) == 0 {
		return false, nil
	}

	added := false
	for _, app := range apps {
		err2 := kubeClient.ApplyFile(ctx, app, acd.Namespace())
		if err2 != nil {
			return added, fmt.Errorf("applying app manifest from %s: %w", app, err2)
		}

		added = true
	}

	return added, nil
}

func recursivelyApplyApps(ctx context.Context, kubeClient *kube.Kube, acd *argocd.ArgoCD, numRecursions *int, processedApps *map[string]struct{}) error {
	*numRecursions++
	if *numRecursions > MaxRecursions {
		return fmt.Errorf("max recursions of %d reached", MaxRecursions)
	}

	added := false
	appList, err := acd.GetAppList(ctx)
	if err != nil {
		return fmt.Errorf("getting app list using argocd: %w", err)
	}

	for _, appListItem := range appList {
		app := strings.TrimSpace(appListItem)
		if app == "" {
			continue
		}

		fmt.Fprintf(os.Stdout, "🍓 Scanning application %s (recursion: %d)...\n", app, *numRecursions)

		// check if app has been already processed
		_, ok := (*processedApps)[app]
		if ok {
			continue
		}

		err := acd.WaitForAppManifests(ctx, app)
		if err != nil {
			return fmt.Errorf("waiting for app %s manifests: %w", app, err)
		}

		manifests, err := acd.GetAppManifests(ctx, app)
		if err != nil {
			return fmt.Errorf("getting app %s manifests: %w", app, err)
		}

		addedApps, err := extractAndApplyAppsFromManifestsYAML(ctx, manifests, kubeClient, acd)
		if err != nil {
			return fmt.Errorf("extracting and applying apps from manifests yaml: %w", err)
		}

		if addedApps {
			added = true
		}

		(*processedApps)[app] = struct{}{}
	}

	if added {
		err := recursivelyApplyApps(ctx, kubeClient, acd, numRecursions, processedApps)
		if err != nil {
			return fmt.Errorf("resursively applying apps (recursion: %d): %w", err)
		}
	}

	return nil
}
