package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Bootstrap(projectName, protoPath string) {
	parts := strings.Split(projectName, "/")
	if len(parts) == 0 {
		CheckErr(errors.New("project name should be fully qualified git repo name"))
	}

	fmt.Printf("Bootstrapping %s\n", projectName)

	folderName, err := initializeGoProject(projectName)
	CheckErr(err)

	err = createProjectStructure(folderName)
	CheckErr(err)

	err = GenerateMain(projectName, folderName)
	CheckErr(err)

	err = GenerateBuildScripts(protoPath, folderName)
	CheckErr(err)

	err = CopyGitIgnore(folderName)
	CheckErr(err)

	err = CopyIniFile(folderName)
	CheckErr(err)

	err = GenerateDockerFile(folderName)
	CheckErr(err)
}

func initializeGoProject(prjName string) (string, error) {
	folderName := filepath.Base(prjName)
	err := os.Mkdir(folderName, os.ModePerm)
	if err != nil {
		return "", err
	}

	err = runGoModInit(prjName, folderName)
	if err != nil {
		return "", err
	}

	return folderName, nil
}

func createProjectStructure(folderName string) error {
	err := os.Mkdir(filepath.Join(folderName, "db"), os.ModePerm)
	if err != nil {
		return err
	}

	err = os.Mkdir(filepath.Join(folderName, "services"), os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
