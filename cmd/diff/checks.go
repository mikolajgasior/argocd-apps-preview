package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mikolajgasior/argocd-apps-preview/pkg/logmsg"
)

func checkPrerequisites() {
	logmsg.Info("Checking prerequisites...")

	_, err := exec.LookPath("git")
	if err != nil {
		logmsg.Error(ErrMsgGitNotFound, nil)
		os.Exit(ExitGitNotFound)
	}
}

func checkDirs(appsBase, appsTarget, outputDiff string) {
	for _, dir := range [4]string{appsBase, appsTarget, outputDiff} {
		if dir == "" {
			continue
		}

		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				logmsg.Error(fmt.Sprintf("%s directory not found: %v", dir, err), nil)
				os.Exit(ExitDirNotFound)
			}
			logmsg.Error(fmt.Sprintf("Error checking %s directory: %v", dir, err), nil)
			os.Exit(ExitDirNotFound)
		}
		if !info.IsDir() {
			logmsg.Error(fmt.Sprintf("%s exists but is not a directory", dir), nil)
			os.Exit(ExitDirNotFound)
		}
	}
	entries, err := os.ReadDir(outputDiff)
	if err != nil {
		logmsg.Error(fmt.Sprintf("Error reading %s directory: %v", outputDiff, err), nil)
		os.Exit(ExitOutputsDirNotFound)
	}
	if len(entries) > 0 {
		logmsg.Error(ErrMsgOutputsDirNotEmpty, nil)
		os.Exit(ExitOutputsDirNotEmpty)
	}
}
