package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Bootstrap(projectName, protoPath string) error {
	parts := strings.Split(projectName, "/")
	if len(parts) == 0 {
		return errors.New("project name should be fully qualified git repo name")
	}

	fmt.Printf("Bootstrapping %s\n", projectName)

	folderName, err := initializeGoProject(projectName)
	if err != nil {
		return err
	}

	err = createProjectStructure(folderName)
	if err != nil {
		return err
	}

	err = GenerateMain(projectName, folderName)
	if err != nil {
		return err
	}

	err = GenerateBuildScripts(protoPath, folderName)
	if err != nil {
		return err
	}

	err = CopyGitIgnore(folderName)
	if err != nil {
		return err
	}

	err = CopyIniFile(folderName)
	if err != nil {
		return err
	}

	err = GenerateDockerFile(folderName)
	if err != nil {
		return err
	}

	err = GenerateLoginService(projectName, folderName)
	if err != nil {
		return err
	}

	err = GenerateLoginRepository(projectName, folderName)
	if err != nil {
		return err
	}

	return nil
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

// Will run inside the project folder
func AddRepository(modelName string) error {
	if strings.Contains(modelName, "Model") || strings.Contains(modelName, "Repo") {
		return errors.New("pass only root name of db model/repo")
	}

	fmt.Printf("Adding repository %s Repository\n", modelName)

	err := GenerateRepo(modelName)
	if err != nil {
		return err
	}

	return nil
}

// Will run inside the project folder
func AddService(serviceName string) error {
	if strings.Contains(serviceName, "Service") {
		return errors.New("pass only root name of service")
	}

	fmt.Printf("Adding service %sService\n", serviceName)

	projectName, err := getProjectName()
	if err != nil {
		return err
	}

	err = GenerateService(serviceName, projectName)
	if err != nil {
		return err
	}

	return nil
}
