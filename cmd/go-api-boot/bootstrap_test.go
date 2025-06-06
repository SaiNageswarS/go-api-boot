package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitializeGoProject(t *testing.T) {
	projectName := "github.com/example/testproject"
	folderName, err := initializeGoProject(projectName)
	defer os.RemoveAll(folderName) // clean up after test

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedFolder := "testproject"
	if folderName != expectedFolder {
		t.Errorf("Expected folder name %s, got %s", expectedFolder, folderName)
	}

	if _, err := os.Stat(folderName); os.IsNotExist(err) {
		t.Errorf("Expected folder %s to be created", folderName)
	}
}

func TestInitializeGoProject_InvalidPath(t *testing.T) {
	invalidName := "/invalid//path"
	defer os.RemoveAll("path")

	_, err := initializeGoProject(invalidName)
	if err == nil {
		t.Error("Expected error for invalid path but got nil")
	}
}

func TestCreateProjectStructure(t *testing.T) {
	testFolder := "teststructure"
	err := os.Mkdir(testFolder, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create base folder: %v", err)
	}
	defer os.RemoveAll(testFolder)

	err = createProjectStructure(testFolder)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	dbPath := filepath.Join(testFolder, "db")
	svcPath := filepath.Join(testFolder, "services")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Expected db folder to be created")
	}

	if _, err := os.Stat(svcPath); os.IsNotExist(err) {
		t.Errorf("Expected services folder to be created")
	}
}

func TestBootstrap_Success(t *testing.T) {
	projectName := "github.com/example/bootproj"
	Bootstrap(projectName, "proto") // Assuming proto path is dummy
	defer os.RemoveAll("bootproj")

	// Just a sanity check that folder was created
	if _, err := os.Stat("bootproj"); os.IsNotExist(err) {
		t.Errorf("Expected folder bootproj to be created")
	}

	// Check LoginService file
	loginServicePath := filepath.Join("bootproj", "services", "loginService.go")
	if _, err := os.Stat(loginServicePath); os.IsNotExist(err) {
		t.Errorf("Expected loginService.go to be created")
	}
}

func TestAddRepository_ErrorName(t *testing.T) {
	// Test with an invalid model name
	err := AddRepository("loginModel")
	assert.Error(t, err, "Expected error for invalid model name")
	assert.EqualError(t, err, "pass only root name of db model/repo")
}

func TestAddRepository_Success(t *testing.T) {
	tmp := t.TempDir()

	// work inside an isolated dir so relative "db" / "services" paths are safe
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	if err := os.Mkdir("db", os.ModePerm); err != nil {
		t.Fatalf("make db: %v", err)
	}

	err := AddRepository("Profile")
	assert.NoError(t, err, "Expected no error when adding repository")
}

func TestAddService_ErrorName(t *testing.T) {
	// Test with an invalid model name
	err := AddService("loginService")
	assert.Error(t, err, "Expected error for invalid model name")
	assert.EqualError(t, err, "pass only root name of service")
}

func TestAddService_getProjectName_Error(t *testing.T) {
	restore := getProjectName
	defer func() { getProjectName = restore }()
	getProjectName = func() (string, error) {
		return "", os.ErrNotExist // simulate missing go.mod
	}

	err := AddService("Profile")
	assert.Error(t, err, "Expected error when project name cannot be determined")
	assert.EqualError(t, err, "file does not exist")
}

func TestAddService_Success(t *testing.T) {
	tmp := t.TempDir()

	// work inside an isolated dir so relative "services" path is safe
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	if err := os.Mkdir("services", os.ModePerm); err != nil {
		t.Fatalf("make services: %v", err)
	}

	restore := getProjectName
	defer func() { getProjectName = restore }()
	getProjectName = func() (string, error) {
		return "github.com/example/testproject", nil
	}

	err := AddService("Profile")
	assert.NoError(t, err, "Expected no error when adding service")
}
