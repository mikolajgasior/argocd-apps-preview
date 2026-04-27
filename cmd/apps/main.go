package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mikolajgasior/argocd-apps-preview/pkg/argocd"
	"github.com/mikolajgasior/argocd-apps-preview/pkg/command"
	"github.com/mikolajgasior/argocd-apps-preview/pkg/kind"
	"github.com/mikolajgasior/argocd-apps-preview/pkg/kube"
	"github.com/mikolajgasior/argocd-apps-preview/pkg/logmsg"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	regexpRepoURL = regexp.MustCompile(`^(https:\/\/|ssh:\/\/|git@)[\w\-\.]+(:|\/)[\w\-\/]+(\.git)?$`)
	regexpGitRef  = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

func main() {
	var outputAppsDir string
	var hooksDir string
	var manifestsDir string
	var secretsDir string
	var repoURL string
	var targetRevision string

	rootCmd := &cobra.Command{
		Use:   "apps",
		Short: "ArgoCD Apps Preview",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if repoURL != "" || targetRevision != "" {
				if !regexpRepoURL.MatchString(repoURL) {
					return fmt.Errorf("invalid repository URL: %s", repoURL)
				}
				if !regexpGitRef.MatchString(targetRevision) {
					return fmt.Errorf("invalid target revision: %s", targetRevision)
				}
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			checkPrerequisites()
			checkDirs(manifestsDir, secretsDir, hooksDir, outputAppsDir)
			execute(manifestsDir, secretsDir, hooksDir, outputAppsDir, repoURL, targetRevision)
		},
	}

	rootCmd.Flags().StringVar(&manifestsDir, "manifests", "", "Directory with start manifests")
	rootCmd.Flags().StringVar(&secretsDir, "secrets", "", "Directory with secrets")
	rootCmd.Flags().StringVar(&hooksDir, "hooks", "", "Directory for hooks scripts")
	rootCmd.Flags().StringVar(&outputAppsDir, "output-apps", "", "Directory to output app manifests")
	rootCmd.Flags().StringVar(&repoURL, "replace-repo-url", "", "Repository URL to replace")
	rootCmd.Flags().StringVar(&targetRevision, "replace-target-revision", "", "Target revision to replace")
	_ = rootCmd.MarkFlagRequired("manifests")
	_ = rootCmd.MarkFlagRequired("outputs")

	err := rootCmd.Execute()
	if err != nil {
		logmsg.Error("Error executing command", err)
		os.Exit(1)
	}
}

func execute(manifestsDir, secretsDir, hooksDir, outputAppsDir, repoURL, revision string) {
	if repoURL != "" && revision != "" {
		logmsg.Info(fmt.Sprintf("Changing repository URL and target revision to: %s %s", repoURL, revision))
	}

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
	if secretsDir != "" {
		err = applyManifests(ctxArgoCD, kubeClient, secretsDir, ArgoCDNamespace)
		if err != nil {
			logmsg.Error(ErrMsgApplyingSecretsFailed, err)
			os.Exit(ExitApplyingSecretsFailed)
		}
	}

	// apply initial manifests (we need to start somewhere)
	err = applyAppManifestsFromDir(ctxArgoCD, kubeClient, acd, manifestsDir, hooksDir, [2]string{repoURL, revision})
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
	err = recursivelyApplyApps(ctxRecursiveApply, kubeClient, acd, &numRecursions, &processedApps, hooksDir, [2]string{repoURL, revision})
	if err != nil {
		logmsg.Error(ErrMsgRecursivelyApplyingAppsFailed, err)
		os.Exit(ExitRecursivelyApplyingAppsFailed)
	}
	logmsg.Info("Finished recursively applying applications.")

	// dump app manifests to the outputs directory
	err = dumpAppManifests(ctxArgoCD, acd, outputAppsDir)
	if err != nil {
		logmsg.Error(ErrMsgDumpingAppManifestsFailed, err)
		os.Exit(ExitDumpingAppManifestsFailed)
	}
	logmsg.Info(fmt.Sprintf("Finished dumping app manifests to %s directory.", outputAppsDir))
}

func getKubeContext() string {
	return fmt.Sprintf("kind-%s", KindName)
}

func executeHookIfExists(ctx context.Context, hookPath string, envVars map[string]string) error {
	info, err := os.Stat(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("getting stat for hook file %s: %w", hookPath, err)
	}
	if info.IsDir() {
		return nil
	}

	logmsg.Info(fmt.Sprintf("Executing hook: %s", hookPath))
	cmd, err := command.NewCommand("bash", hookPath)
	if err != nil {
		return fmt.Errorf("creating command for hook %s: %w", hookPath, err)
	}
	err = cmd.Run(ctx, &envVars)
	if err != nil {
		return fmt.Errorf("executing hook %s: %w", hookPath, err)
	}
	return nil
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

func applyAppManifestsFromDir(ctx context.Context, kubeClient *kube.Kube, acd *argocd.ArgoCD, dir string, hooksDir string, target [2]string) error {
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
			_, err2 := extractAndApplyAppsFromManifestsYAML(ctx, path, kubeClient, acd, hooksDir, target)
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

func extractAndApplyAppsFromManifestsYAML(ctx context.Context, path string, kubeClient *kube.Kube, acd *argocd.ArgoCD, hooksDir string, target [2]string) (bool, error) {
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
			hookPath := filepath.Join(hooksDir, "before-appset-gen.sh")
			err := executeHookIfExists(ctx, hookPath, map[string]string{
				"APPSET_YAML": appSet,
			})
			if err != nil {
				return false, fmt.Errorf("executing before-appset-gen hook: %w", err)
			}

			if target[0] != "" && target[1] != "" {
				modifiedAppSet, err := replaceRepoURLAndTargetRevision(appSet, target[0], target[1])
				if err != nil {
					return false, fmt.Errorf("replacing repo URL and target revision in appset %s: %w", appSet, err)
				}

				appSet = modifiedAppSet
			}

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
		hookPath := filepath.Join(hooksDir, "before-app-apply.sh")
		err := executeHookIfExists(ctx, hookPath, map[string]string{
			"APP_YAML": app,
		})
		if err != nil {
			return false, fmt.Errorf("executing before-app-apply hook: %w", err)
		}

		if target[0] != "" && target[1] != "" {
			modifiedApp, err := replaceRepoURLAndTargetRevision(app, target[0], target[1])
			if err != nil {
				return false, fmt.Errorf("replacing repo URL and target revision in appset %s: %w", app, err)
			}

			app = modifiedApp
		}

		err2 := kubeClient.ApplyFile(ctx, app, acd.Namespace())
		if err2 != nil {
			return added, fmt.Errorf("applying app manifest from %s: %w", app, err2)
		}

		added = true
	}

	return added, nil
}

func recursivelyApplyApps(ctx context.Context, kubeClient *kube.Kube, acd *argocd.ArgoCD, numRecursions *int, processedApps *map[string]struct{}, hooksDir string, target [2]string) error {
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

		// check if the app has been already processed
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

		addedApps, err := extractAndApplyAppsFromManifestsYAML(ctx, manifests, kubeClient, acd, hooksDir, target)
		if err != nil {
			return fmt.Errorf("extracting and applying apps from manifests yaml: %w", err)
		}

		if addedApps {
			added = true
		}

		(*processedApps)[app] = struct{}{}
	}

	if added {
		err := recursivelyApplyApps(ctx, kubeClient, acd, numRecursions, processedApps, hooksDir, target)
		if err != nil {
			return fmt.Errorf("recursively applying apps (recursion: %d): %w", *numRecursions, err)
		}
	}

	return nil
}

func replaceRepoURLAndTargetRevision(appSet string, repoURL string, targetRevision string) (string, error) {
	logmsg.Info(fmt.Sprintf("Changing target revision to %s in repository %s...", targetRevision, repoURL))

	appSetYAML, err := os.ReadFile(appSet)
	if err != nil {
		return "", fmt.Errorf("reading appset %s: %w", appSet, err)
	}

	var rootNode yaml.Node
	err = yaml.Unmarshal(appSetYAML, &rootNode)
	if err != nil {
		return "", fmt.Errorf("parsing YAML document: %w", err)
	}

	updated := false
	var traverse func(node *yaml.Node) error
	traverse = func(node *yaml.Node) error {
		if node == nil {
			return nil
		}

		if node.Kind != yaml.MappingNode {
			for _, child := range node.Content {
				err := traverse(child)
				if err != nil {
					return err
				}
			}

			return nil
		}

		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			value := node.Content[i+1]

			if key.Value == "repoURL" && value.Value == repoURL {
				logmsg.Info(fmt.Sprintf("Found key %s with value %s...", key.Value, value.Value))
				for j := 0; j < len(node.Content); j += 2 {
					siblingKey := node.Content[j]
					siblingValue := node.Content[j+1]

					if siblingKey.Value == "revision" || siblingKey.Value == "targetRevision" {
						logmsg.Info(fmt.Sprintf("Found sibling key %s with value %s...", siblingKey.Value, siblingValue.Value))
						siblingValue.Value = targetRevision
						updated = true
					}
				}
			}

			err := traverse(value)
			if err != nil {
				return err
			}
		}

		return nil
	}

	err = traverse(&rootNode)
	if err != nil {
		return "", fmt.Errorf("traversing YAML document: %w", err)
	}

	if !updated {
		return appSet, nil
	}

	logmsg.Info(fmt.Sprintf("Changed targetRevision in %s...", appSet))
	newFileName := filepath.Join(filepath.Dir(appSet), "B_"+filepath.Base(appSet))
	newYAML, err := yaml.Marshal(&rootNode)
	if err != nil {
		return "", fmt.Errorf("encoding updated YAML: %w", err)
	}

	err = os.WriteFile(newFileName, newYAML, 0644)
	if err != nil {
		return "", fmt.Errorf("writing updated YAML to file %s: %w", newFileName, err)
	}

	return newFileName, nil

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
