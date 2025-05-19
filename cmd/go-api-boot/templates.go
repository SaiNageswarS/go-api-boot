package main

import (
	"embed"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*
var templatesFS embed.FS

func GenerateMain(projectPath, folderName string) error {
	data := map[string]string{
		"ProjectPath": projectPath,
	}

	return generateCode(folderName, "templates/main.go.tmpl", "main.go", data)
}

func GenerateBuildScripts(protoPath, folderName string) error {
	data := map[string]string{
		"FolderName": folderName,
		"ProtoPath":  protoPath,
	}

	err := generateCode(folderName, "templates/build.ps1.tmpl", "build.ps1", data)
	if err != nil {
		return err
	}

	return generateCode(folderName, "templates/build.sh.tmpl", "build.sh", data)
}

func GenerateDockerFile(folderName string) error {
	return generateCode(folderName, "templates/Dockerfile.tmpl", "Dockerfile", map[string]string{"ExeName": folderName})
}

func CopyGitIgnore(folderName string) error {
	return generateCode(folderName, "templates/.gitignore.tmpl", ".gitignore", map[string]string{})
}

func GenerateRepo(modelName string) error {
	data := map[string]string{
		"ModelName": modelName,
	}

	return generateCode("db", "templates/repo.go.tmpl", modelName+"Repository.go", data)
}

func GenerateService(serviceName string, projectPath string) error {
	data := map[string]string{
		"ServiceName": serviceName,
		"ProjectPath": projectPath,
	}

	return generateCode("services", "templates/services.go.tmpl", serviceName+"Service.go", data)
}

func generateCode(folderName, templatePath, fileName string, templateData interface{}) error {
	tmpl, err := template.ParseFS(templatesFS, templatePath)
	if err != nil {
		return err
	}

	file, err := os.Create(filepath.Join(folderName, fileName))
	if err != nil {
		return err
	}

	defer file.Close()
	return tmpl.Execute(file, templateData)
}
