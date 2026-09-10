package components

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewExternalEditorTarget_UnsetDirUsesTheAppDataDirectory(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	target, err := newExternalEditorTarget("", "postgres://user:pass@localhost:5432/shopdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantDir := filepath.Join(configHome, "lazysql", externalEditorDirName)
	if target.Workdir != wantDir {
		t.Errorf("expected the file under %s, got %s", wantDir, target.Path)
	}
	if target.Cleanup {
		t.Error("expected the app data file to be kept between sessions")
	}
	if !strings.HasSuffix(target.Path, ".sql") {
		t.Errorf("expected a .sql file, got %s", target.Path)
	}
	if strings.Contains(filepath.Base(target.Path), "/") || strings.Contains(filepath.Base(target.Path), ":") {
		t.Errorf("expected the connection to be sanitized into the file name, got %s", target.Path)
	}
}

func TestNewExternalEditorTarget_AppDataFileIsPerConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	first, err := newExternalEditorTarget("", "postgres://localhost/one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := newExternalEditorTarget("", "postgres://localhost/two")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first.Path == second.Path {
		t.Errorf("expected different connections to get different files, both got %s", first.Path)
	}
}

func TestNewExternalEditorTarget_AppDataFileIsStablePerConnection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	first, err := newExternalEditorTarget("", "postgres://localhost/one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := newExternalEditorTarget("", "postgres://localhost/one")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first.Path != second.Path {
		t.Errorf("expected a stable path, got %s then %s", first.Path, second.Path)
	}
}

func TestNewExternalEditorTarget_UnknownConnectionStillGetsAFile(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	target, err := newExternalEditorTarget("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(configHome, "lazysql", externalEditorDirName, externalEditorFileName)
	if target.Path != want {
		t.Errorf("expected %s, got %s", want, target.Path)
	}
}

func TestNewExternalEditorTarget_ConfiguredDirGetsAStableFile(t *testing.T) {
	dir := t.TempDir()

	target, err := newExternalEditorTarget(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := filepath.Join(dir, externalEditorFileName); target.Path != want {
		t.Errorf("expected %s, got %s", want, target.Path)
	}
	if target.Workdir != dir {
		t.Errorf("expected the editor to run in %s, got %s", dir, target.Workdir)
	}
	if target.Cleanup {
		t.Error("expected a configured file to be kept between sessions")
	}
}

func TestNewExternalEditorTarget_SameFileEveryTime(t *testing.T) {
	dir := t.TempDir()

	first, err := newExternalEditorTarget(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := newExternalEditorTarget(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if first.Path != second.Path {
		t.Errorf("expected a stable path, got %s then %s", first.Path, second.Path)
	}
}

func TestNewExternalEditorTarget_CreatesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "queries")

	target, err := newExternalEditorTarget(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info, err := os.Stat(filepath.Dir(target.Path)); err != nil || !info.IsDir() {
		t.Errorf("expected %s to be created: %v", dir, err)
	}
}

func TestNewExternalEditorTarget_ExpandsHome(t *testing.T) {
	// A stand-in home: the real one must not be touched, and sandboxed builds
	// point HOME at a directory that does not exist.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on Windows

	target, err := newExternalEditorTarget("~/queries", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := filepath.Join(home, "queries", externalEditorFileName); target.Path != want {
		t.Errorf("expected %s, got %s", want, target.Path)
	}
}

func TestNewExternalEditorTarget_ResolvesRelativePathsAgainstTheWorkingDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	target, err := newExternalEditorTarget("queries", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !filepath.IsAbs(target.Path) {
		t.Errorf("expected an absolute path, got %s", target.Path)
	}
	if filepath.Base(filepath.Dir(target.Path)) != "queries" {
		t.Errorf("expected the file inside ./queries, got %s", target.Path)
	}
}

func TestNewExternalEditorTarget_FailsWhenTheDirectoryIsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := newExternalEditorTarget(path, ""); err == nil {
		t.Error("expected an error when the configured directory is a regular file")
	}
}
