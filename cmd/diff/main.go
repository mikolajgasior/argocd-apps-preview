package main

import (
	"os"

	"github.com/mikolajgasior/argocd-apps-preview/pkg/logmsg"
	"github.com/spf13/cobra"
)

func main() {
	var appsBaseDir string
	var appsTargetDir string
	var outputDiffDir string

	rootCmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff apps",
		Run: func(cmd *cobra.Command, args []string) {
			checkPrerequisites()
			checkDirs(appsBaseDir, appsTargetDir, outputDiffDir)
			execute(appsBaseDir, appsTargetDir, outputDiffDir)
		},
	}

	err := rootCmd.Execute()
	if err != nil {
		logmsg.Error("Error executing command", err)
		os.Exit(1)
	}
}

func execute(appsBaseDir, appsTargetDir, outputDiffDir string) {

}
