package portscan

import (
	"os"
	"path/filepath"
	"testing"
)

func writePackageJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectNameFromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writePackageJSON(t, dir, `{"name": "my-api", "version": "1.0.0"}`)

	if got := ProjectName(dir); got != "my-api" {
		t.Fatalf("ProjectName() = %q, want %q", got, "my-api")
	}
}

func TestProjectNameFallsBackToDirectoryName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkout-service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := ProjectName(dir); got != "checkout-service" {
		t.Fatalf("ProjectName() = %q, want %q", got, "checkout-service")
	}
}

func TestProjectNameFallsBackWhenPackageJSONHasNoName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-name-app")
	writePackageJSON(t, dir, `{"private": true}`)

	if got := ProjectName(dir); got != "no-name-app" {
		t.Fatalf("ProjectName() = %q, want %q", got, "no-name-app")
	}
}

func TestProjectNameFallsBackOnInvalidJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "broken-app")
	writePackageJSON(t, dir, `{"name": `)

	if got := ProjectName(dir); got != "broken-app" {
		t.Fatalf("ProjectName() = %q, want %q", got, "broken-app")
	}
}

func TestProjectNameFindsPackageJSONInParentDirectory(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name": "monorepo-web"}`)
	nested := filepath.Join(root, "src", "server")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := ProjectName(nested); got != "monorepo-web" {
		t.Fatalf("ProjectName() = %q, want %q", got, "monorepo-web")
	}
}

func TestProjectNamePrefersNearestPackageJSON(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name": "monorepo-root"}`)
	pkg := filepath.Join(root, "packages", "worker")
	writePackageJSON(t, pkg, `{"name": "@acme/worker"}`)

	if got := ProjectName(pkg); got != "@acme/worker" {
		t.Fatalf("ProjectName() = %q, want %q", got, "@acme/worker")
	}
}

func TestProjectNameStopsSearchingAfterMaxDepth(t *testing.T) {
	root := t.TempDir()
	writePackageJSON(t, root, `{"name": "too-far-up"}`)
	deep := filepath.Join(root, "a", "b", "c", "d", "e", "f")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := ProjectName(deep); got != "f" {
		t.Fatalf("ProjectName() = %q, want %q", got, "f")
	}
}

func TestProjectNameRootCwdHasNoProject(t *testing.T) {
	if got := ProjectName("/"); got != "" {
		t.Fatalf("ProjectName(\"/\") = %q, want empty string", got)
	}
}

func TestProjectNameEmptyCwd(t *testing.T) {
	if got := ProjectName(""); got != "" {
		t.Fatalf("ProjectName(\"\") = %q, want empty string", got)
	}
}
