package components

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jorgerojas26/lazysql/internal/saved"
)

const (
	// externalEditorFileName is the scratch file written into a configured
	// ExternalEditorDir. The name is stable so the editor keeps its session
	// state — undo history, marks, and any assistant running against that
	// project. It is also the fallback name when the connection is unknown.
	externalEditorFileName = "lazysql-query.sql"

	// externalEditorDirName holds the per-connection scratch files, next to the
	// query history and saved queries in lazysql's own data directory.
	externalEditorDirName = "queries"
)

// externalEditorTarget describes the file the external editor is opened on.
type externalEditorTarget struct {
	// Path is the file handed to the editor.
	Path string
	// Workdir is the working directory the editor runs in. Empty means the
	// editor inherits lazysql's own.
	Workdir string
	// Cleanup reports whether the file is throwaway and should be removed once
	// the editor exits.
	Cleanup bool
}

// newExternalEditorTarget decides where the query the external editor works on
// lives. By default that is a per-connection file in lazysql's own data
// directory, out of sight but at a stable path. Pointing ExternalEditorDir at a
// project directory instead puts both the file and the editor's working
// directory inside it, which is what editor plugins that look for a project
// root need.
func newExternalEditorTarget(dir, connectionURL string) (externalEditorTarget, error) {
	if strings.TrimSpace(dir) == "" {
		return appDataEditorTarget(connectionURL)
	}

	resolved, err := resolveDir(dir)
	if err != nil {
		return externalEditorTarget{}, err
	}

	if err := os.MkdirAll(resolved, 0o750); err != nil {
		return externalEditorTarget{}, fmt.Errorf("failed to create %s: %w", resolved, err)
	}

	return externalEditorTarget{
		Path:    filepath.Join(resolved, externalEditorFileName),
		Workdir: resolved,
	}, nil
}

// appDataEditorTarget puts the file in lazysql's own data directory, one per
// connection, so it stays out of sight but keeps a stable path across sessions.
// It falls back to a throwaway temp file if that directory is unusable.
func appDataEditorTarget(connectionURL string) (externalEditorTarget, error) {
	appDir, err := saved.GetAppConfigDir()
	if err != nil {
		return temporaryEditorTarget()
	}

	dir := filepath.Join(appDir, externalEditorDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return temporaryEditorTarget()
	}

	name := externalEditorFileName
	if strings.TrimSpace(connectionURL) != "" {
		name = saved.SanitizeFilename(connectionURL) + ".sql"
	}

	return externalEditorTarget{
		Path:    filepath.Join(dir, name),
		Workdir: dir,
	}, nil
}

// temporaryEditorTarget is the last resort: a throwaway file removed once the
// editor exits.
func temporaryEditorTarget() (externalEditorTarget, error) {
	file, err := os.CreateTemp("", "lazysql-*.sql")
	if err != nil {
		return externalEditorTarget{}, fmt.Errorf("failed to create temporary file: %w", err)
	}

	if err := file.Close(); err != nil {
		return externalEditorTarget{}, fmt.Errorf("failed to close temporary file: %w", err)
	}

	return externalEditorTarget{Path: file.Name(), Cleanup: true}, nil
}

// resolveDir expands a leading "~" and turns a relative path into an absolute
// one, so a configured directory means the same thing wherever lazysql runs
// from.
func resolveDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)

	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to expand %q: %w", dir, err)
		}

		dir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(dir, "~"), "/"))
	}

	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %q: %w", dir, err)
	}

	return absolute, nil
}
