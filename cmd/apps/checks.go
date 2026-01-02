package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/keenbytes/argocd-apps-preview/pkg/logmsg"
)

func checkPrerequisites() {
	logmsg.Info("Checking prerequisites...")

	_, err := exec.LookPath("kind")
	if err != nil {
		logmsg.Error(ErrMsgKindNotFound, nil)
		os.Exit(ExitKindNotFound)
	}

	_, err = exec.LookPath("argocd")
	if err != nil {
		logmsg.Error(ErrMsgArgoCDNotFound, nil)
		os.Exit(ExitArgoCDNotFound)
	}

	_, err = exec.LookPath("kubectl")
	if err != nil {
		logmsg.Error(ErrMsgKubectlNotFound, nil)
		os.Exit(ExitKubectlNotFound)
	}
}

func checkManifestsDir() {
	info, err := os.Stat(DirManifests)
	if err != nil {
		if os.IsNotExist(err) {
			logmsg.Error(ErrMsgManifestsDirNotFound, nil)
			os.Exit(ExitManifestsDirNotFound)
		}
		logmsg.Error(fmt.Sprintf("Error checking %s directory: %v", DirManifests, err), nil)
		os.Exit(ExitManifestsDirNotFound)
	}
	if !info.IsDir() {
		logmsg.Error(fmt.Sprintf("%s exists but is not a directory", DirManifests), nil)
		os.Exit(ExitManifestsDirNotFound)
	}
}

func checkOutputsDir() {
	info, err := os.Stat(DirOutputs)
	if err != nil {
		if os.IsNotExist(err) {
			logmsg.Error(ErrMsgOutputsDirNotFound, nil)
			os.Exit(ExitOutputsDirNotFound)
		}
		logmsg.Error(fmt.Sprintf("Error checking %s directory: %v", DirOutputs, err), nil)
		os.Exit(ExitOutputsDirNotFound)
	}
	if !info.IsDir() {
		logmsg.Error(fmt.Sprintf("%s exists but is not a directory", DirOutputs), nil)
		os.Exit(ExitOutputsDirNotFound)
	}

	entries, err := os.ReadDir(DirOutputs)
	if err != nil {
		logmsg.Error(fmt.Sprintf("Error reading %s directory: %v", DirOutputs, err), nil)
		os.Exit(ExitOutputsDirNotFound)
	}
	if len(entries) > 0 {
		logmsg.Error(ErrMsgOutputsDirNotEmpty, nil)
		os.Exit(ExitOutputsDirNotEmpty)
	}
}
