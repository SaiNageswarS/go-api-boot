package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	bootstrapFn  = Bootstrap
	addRepoFn    = AddRepository
	addServiceFn = AddService
)

func main() {
	if err := NewRoot().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "go-api-boot",
		Short: "A CLI to bootstrap go-api-boot gRPC microservice project",
	}

	root.AddCommand(&cobra.Command{
		Use:   "bootstrap [project name] [proto path]",
		Short: "Bootstrap a new go-api-boot gRPC microservice project",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return bootstrapFn(args[0], args[1])
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "repository [dbModelName]",
		Short: "Add DB repository",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return addRepoFn(args[0]) },
	})
	root.AddCommand(&cobra.Command{
		Use:   "service [serviceName]",
		Short: "Add service API",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return addServiceFn(args[0]) },
	})
	return root
}
