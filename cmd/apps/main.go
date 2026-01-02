package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keenbytes/argocd-apps-preview/pkg/argocd"
	"github.com/keenbytes/argocd-apps-preview/pkg/kind"
	"github.com/keenbytes/argocd-apps-preview/pkg/kube"
	"github.com/keenbytes/argocd-apps-preview/pkg/logmsg"
)

func main() {
	// check required software and necessary local directories
	checkPrerequisites()
	checkManifestsDir()
	checkOutputsDir()

	// start kind cluster
	cluster := kind.NewKind(KindName, KindImage)
	_ = cluster.Delete()

	ctxCluster, cancelCluster := context.WithTimeout(context.Background(), CtxClusterTimeoutSeconds*time.Second)
	defer cancelCluster()

	err := cluster.Create(ctxCluster)
	if err != nil {
		logmsg.Error(ErrMsgCreatingClusterFailed, err)
		_ = cluster.Delete()
		os.Exit(ExitCreatingClusterFailed)
	}
	defer cluster.Delete()

	// get kube client
	kubeClient := kube.NewKube(getKubeContext())

	// install argocd on the kind cluster
	acd := argocd.NewArgoCD(kubeClient, ArgoCDNamespace, ArgoCDVersion, ArgoCDNodePort)

	// add timeout to context for argocd installation
	ctxArgoCD, cancelArgoCD := context.WithTimeout(context.Background(), CtxArgoCDTimeoutSeconds*time.Second)
	defer cancelArgoCD()

	err = acd.Install(ctxArgoCD)
	if err != nil {
		logmsg.Error(ErrMsgArgoCDInstallationFailed, err)
		os.Exit(ExitArgoCDInstallationFailed)
	}

	// wait a while until argocd starts up
	time.Sleep(SleepSecondsAfterArgoCDInstall * time.Second)

	// log in to argocd
	err = acd.Login(ctxArgoCD)
	if err != nil {
		logmsg.Error(ErrMsgArgoCDLoggingFailed, err)
		os.Exit(ExitArgoCDLoggingFailed)
	}

	// apply manifests from the secrets (to allow argocd pull from private repositories etc.)
	err = applyManifests(ctxArgoCD, kubeClient, DirSecrets, ArgoCDNamespace)
	if err != nil {
		logmsg.Error(ErrMsgApplyingSecretsFailed, err)
		os.Exit(ExitApplyingSecretsFailed)
	}

	// apply initial manifests (we need to start somewhere)
	err = applyAppManifestsFromDir(ctxArgoCD, kubeClient, acd, DirManifests)
	if err != nil {
		logmsg.Error(ErrMsgApplyingManifestsFailed, err)
		os.Exit(ExitApplyingManifestsFailed)
	}

	// add timeout to context for getting the applications recursively
	ctxRecursiveApply, cancelRecursiveApply := context.WithTimeout(context.Background(), 360*time.Second)
	defer cancelRecursiveApply()

	// process argocd applications recursively
	logmsg.Info("Starting to recursively apply applications...")
	numRecursions := 0
	processedApps := map[string]struct{}{}
	err = recursivelyApplyApps(ctxRecursiveApply, kubeClient, acd, &numRecursions, &processedApps)
	if err != nil {
		logmsg.Error(ErrMsgRecursivelyApplyingAppsFailed, err)
		os.Exit(ExitRecursivelyApplyingAppsFailed)
	}
	logmsg.Info("Finished recursively applying applications.")

	// dump app manifests to the outputs directory
	err = dumpAppManifests(ctxArgoCD, acd, DirOutputs)
	if err != nil {
		logmsg.Error(ErrMsgDumpingAppManifestsFailed, err)
		os.Exit(ExitDumpingAppManifestsFailed)
	}
	logmsg.Info(fmt.Sprintf("Finished dumping app manifests to %s directory.", DirOutputs))
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

		logmsg.Info(fmt.Sprintf("Scanning application %s (recursion: %d)...", app, *numRecursions))

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
			return fmt.Errorf("recursively applying apps (recursion: %d): %w", *numRecursions, err)
		}
	}

	return nil
}

func dumpAppManifests(ctx context.Context, acd *argocd.ArgoCD, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("output directory %s does not exist", dir)
		}
		return fmt.Errorf("getting stat for directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading output directory %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("output directory %s is not empty", dir)
	}

	appList, err := acd.GetAppList(ctx)
	if err != nil {
		return fmt.Errorf("getting app list using argocd: %w", err)
	}

	for _, appListItem := range appList {
		app := strings.TrimSpace(appListItem)
		if app == "" {
			continue
		}

		err := acd.WaitForAppManifests(ctx, app)
		if err != nil {
			return fmt.Errorf("waiting for app %s manifests: %w", app, err)
		}

		manifestsFile, err := acd.GetAppManifests(ctx, app)
		if err != nil {
			return fmt.Errorf("getting app %s manifests: %w", app, err)
		}

		outputFilename := strings.ReplaceAll(app, "/", "__")

		// copy manifestsFile to dir/outputFilename.yaml
		srcFile, err := os.Open(manifestsFile)
		if err != nil {
			return fmt.Errorf("opening app manifests file %s: %w", manifestsFile, err)
		}
		defer func() {
			err := srcFile.Close()
			if err != nil {
				logmsg.Error("error closing app manifests file "+manifestsFile, err)
			}
		}()

		dstPath := filepath.Join(dir, outputFilename+".yaml")
		dstFile, err := os.Create(dstPath)
		if err != nil {
			err2 := srcFile.Close()
			if err2 != nil {
				return fmt.Errorf("closing manifests file %s: %w", dstPath, err2)
			}
			return fmt.Errorf("creating dump manifests file %s: %w", dstPath, err)
		}
		defer func() {
			err := dstFile.Close()
			if err != nil {
				logmsg.Error("error closing dump manifests file "+manifestsFile, err)
			}
		}()

		_, err = io.Copy(dstFile, srcFile)
		if err != nil {
			return fmt.Errorf("copying file from %s to %s: %w", manifestsFile, dstPath, err)
		}

		err = dstFile.Sync()
		if err != nil {
			return fmt.Errorf("writing app %s manifests to file: %w", app, err)
		}
	}
	return nil
}
