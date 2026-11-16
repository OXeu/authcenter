package server

import (
	"os"
	"path/filepath"
	"testing"
)

func runWithScratch(m *testing.M) int {
	root, err := repoRoot()
	if err != nil {
		println("scratch dir: " + err.Error())
		return 1
	}
	dir := filepath.Join(root, ".tmp", "go-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		println("scratch dir: " + err.Error())
		return 1
	}
	if err := os.Setenv("TMPDIR", dir); err != nil {
		println("scratch dir: " + err.Error())
		return 1
	}
	return m.Run()
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
