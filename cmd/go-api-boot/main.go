package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "go-api-boot",
		Short: "A CLI to bootstrap go-api-boot gRPC microservice project",
	}

	var bootstrapCmd = &cobra.Command{
		Use:   "bootstrap [project name] [proto path]",
		Short: "Bootstrap a new go-api-boot gRPC microservice project",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			Bootstrap(args[0], args[1])
		},
	}

	var addRepositoryCmd = &cobra.Command{
		Use:   "repository [dbModelName]",
		Short: "Add DB repository",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			AddRepository(args[0])
		},
	}

	rootCmd.AddCommand(bootstrapCmd)
	rootCmd.AddCommand(addRepositoryCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runGoModInit(projectName, folderName string) error {
	cmd := exec.Command("go", "mod", "init", projectName)
	cmd.Dir = filepath.Join(".", folderName) // Set the directory for the command
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
