package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Sanity tests to check generation runs without error and creates files.
// These tests do not check the content of the generated files, only that they
// are created and do not return an error. The content is checked in
// templates_test.go.

func TestGenerateMain_OK(t *testing.T) {
	tmp := t.TempDir()
	const pkg = "github.com/example/project"

	if err := GenerateMain(pkg, tmp); err != nil {
		t.Fatalf("GenerateMain returned error: %v", err)
	}

	out := filepath.Join(tmp, "main.go")
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("main.go not created: %v", err)
	}

	src := mustRead(t, out)
	// template should have expanded something – at least package main should be present
	wantContains(t, src, "package main")
}

func TestGenerateBuildScripts_OK(t *testing.T) {
	tmp := t.TempDir()

	if err := GenerateBuildScripts("proto", tmp); err != nil {
		t.Fatalf("GenerateBuildScripts error: %v", err)
	}

	ps1 := filepath.Join(tmp, "build.ps1")
	sh := filepath.Join(tmp, "build.sh")

	if _, err := os.Stat(ps1); err != nil {
		t.Fatalf("build.ps1 not created: %v", err)
	}
	if _, err := os.Stat(sh); err != nil {
		t.Fatalf("build.sh not created: %v", err)
	}

	shSrc := mustRead(t, sh)
	wantContains(t, shSrc, "proto")            // {{.ProtoPath}}
	wantContains(t, shSrc, filepath.Base(tmp)) // {{.FolderName}}
}

func TestGenerateDockerAndGitIgnore_OK(t *testing.T) {
	tmp := t.TempDir()

	if err := GenerateDockerFile(tmp); err != nil {
		t.Fatalf("GenerateDockerFile: %v", err)
	}
	if err := CopyGitIgnore(tmp); err != nil {
		t.Fatalf("CopyGitIgnore: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "Dockerfile")); err != nil {
		t.Fatalf("Dockerfile not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".gitignore")); err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
}

func TestGenerateRepoAndService_OK(t *testing.T) {
	tmp := t.TempDir()

	// work inside an isolated dir so relative "db" / "services" paths are safe
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	_ = os.Chdir(tmp)

	if err := os.Mkdir("db", os.ModePerm); err != nil {
		t.Fatalf("make db: %v", err)
	}
	if err := os.Mkdir("services", os.ModePerm); err != nil {
		t.Fatalf("make services: %v", err)
	}

	if err := GenerateRepo("User"); err != nil {
		t.Fatalf("GenerateRepo: %v", err)
	}
	if err := GenerateService("Auth", "github.com/example/project"); err != nil {
		t.Fatalf("GenerateService: %v", err)
	}

	if _, err := os.Stat(filepath.Join("db", "UserRepository.go")); err != nil {
		t.Fatalf("UserRepository.go not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join("services", "AuthService.go")); err != nil {
		t.Fatalf("AuthService.go not created: %v", err)
	}
}

func TestGenerateCode_DirectoryDoesNotExist(t *testing.T) {
	tmp := t.TempDir()
	badDir := filepath.Join(tmp, "does-not-exist")

	err := generateCode(badDir, "templates/main.go.tmpl", "main.go", nil)
	if err == nil {
		t.Fatalf("expected error when directory is missing")
	}
}

func TestTemplatesParse(t *testing.T) {
	// A very small sanity check that embedded templates compile.
	for _, path := range []string{
		"templates/main.go.tmpl",
		"templates/build.sh.tmpl",
		"templates/build.ps1.tmpl",
		"templates/repo.go.tmpl",
	} {
		if _, err := templatesFS.Open(path); err != nil {
			t.Fatalf("missing embedded template %s (go:embed broken?): %v", path, err)
		}
	}
}

func TestBuildScriptsLineEndings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not relevant on Windows by default")
	}
	tmp := t.TempDir()
	if err := GenerateBuildScripts("proto", tmp); err != nil {
		t.Fatalf("GenerateBuildScripts error: %v", err)
	}
	shSrc := mustRead(t, filepath.Join(tmp, "build.sh"))
	if strings.Contains(shSrc, "\r\n") {
		t.Errorf("build.sh should use Unix line endings")
	}
}

func mustRead(t *testing.T, fn string) string {
	t.Helper()
	b, err := os.ReadFile(fn)
	if err != nil {
		t.Fatalf("cannot read %s: %v", fn, err)
	}
	return string(b)
}

func wantContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}
