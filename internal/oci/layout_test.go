package oci

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyDirectorySkipsMatchingReadOnlyFiles(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	dstDir := filepath.Join(t.TempDir(), "dst")
	filePath := filepath.Join("blobs", "sha256", "blob")
	if err := os.MkdirAll(filepath.Join(srcDir, filepath.Dir(filePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dstDir, filepath.Dir(filePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("same-content")
	if err := os.WriteFile(filepath.Join(srcDir, filePath), data, 0o444); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dstDir, filePath)
	if err := os.WriteFile(target, data, 0o444); err != nil {
		t.Fatal(err)
	}

	if err := copyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("copyDirectory() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("unexpected target contents %q", string(got))
	}
}

func TestCopyDirectoryReplacesChangedReadOnlyFiles(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	dstDir := filepath.Join(t.TempDir(), "dst")
	filePath := filepath.Join("blobs", "sha256", "blob")
	if err := os.MkdirAll(filepath.Join(srcDir, filepath.Dir(filePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dstDir, filepath.Dir(filePath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, filePath), []byte("new-content"), 0o444); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dstDir, filePath)
	if err := os.WriteFile(target, []byte("old-content"), 0o444); err != nil {
		t.Fatal(err)
	}

	if err := copyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("copyDirectory() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-content" {
		t.Fatalf("unexpected target contents %q", string(got))
	}
}
