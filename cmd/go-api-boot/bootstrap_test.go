package main

import (
	"os"
	"path/filepath"
	"testing"
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
