// Package golden compares test output with golden files. Set UPDATE_GOLDEN=1
// to rewrite the golden files instead (then review the diff).
package golden

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Update reports whether golden files should be rewritten.
func Update() bool { return os.Getenv("UPDATE_GOLDEN") == "1" }

// Check compares got with the contents of path, or rewrites path when updating.
func Check(t *testing.T, path string, got []byte) {
	t.Helper()
	if Update() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden file (run with UPDATE_GOLDEN=1 to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s (run with UPDATE_GOLDEN=1 to update)\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}
