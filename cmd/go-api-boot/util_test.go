package main

import (
	"os"
	"path/filepath"
	"testing"
)

// helper to switch into a temp dir and restore afterwards.
func inTempDir(t *testing.T) func() {
	t.Helper()

	oldwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	return func() { _ = os.Chdir(oldwd) }
}

// -----------------------------------------------------------------------------
// 1. happy-path – module directive present
// -----------------------------------------------------------------------------
func TestGetProjectName_Success(t *testing.T) {
	defer inTempDir(t)()

	goMod := `
// Comment
module github.com/example/demo

go 1.22
`
	if err := os.WriteFile("go.mod", []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	name, err := GetProjectName()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "github.com/example/demo" {
		t.Fatalf("got %q, want %q", name, "github.com/example/demo")
	}
}

// -----------------------------------------------------------------------------
// 2. missing go.mod – expect error
// -----------------------------------------------------------------------------
func TestGetProjectName_NoFile(t *testing.T) {
	defer inTempDir(t)() // empty dir, no go.mod

	_, err := GetProjectName()
	if err == nil {
		t.Fatalf("expected error for missing go.mod, got nil")
	}
	if !os.IsNotExist(err) && filepath.Ext(err.Error()) != "" { // basic sanity
		t.Logf("error as expected: %v", err)
	}
}

// -----------------------------------------------------------------------------
// 3. malformed go.mod – no module directive
// -----------------------------------------------------------------------------
func TestGetProjectName_NoModuleDirective(t *testing.T) {
	defer inTempDir(t)()

	badMod := `
go 1.22
require example.com/foo v1.0.0
`
	if err := os.WriteFile("go.mod", []byte(badMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	_, err := GetProjectName()
	if err == nil {
		t.Fatalf("expected error for missing module directive, got nil")
	}
}
