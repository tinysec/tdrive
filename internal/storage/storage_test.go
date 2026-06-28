package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"tdrive/internal/storage"
)

// TestSymlinkEscapeBlocked verifies the os.Root sandbox refuses to follow a
// symlink that points outside the data root — protection that lexical path
// cleaning alone cannot provide.
func TestSymlinkEscapeBlocked(t *testing.T) {

	var rootDir string = t.TempDir()
	var outsideDir string = t.TempDir()

	var secret string = filepath.Join(outsideDir, "secret.txt")

	var err error = os.WriteFile(secret, []byte("top secret"), 0o600)
	if nil != err {
		t.Fatalf("write secret: %v", err)
	}

	// A symlink inside the root pointing at the outside secret.
	err = os.Symlink(secret, filepath.Join(rootDir, "escape"))
	if nil != err {
		t.Skipf("create symlink not permitted: %v", err)
	}

	var store *storage.Store

	store, _, err = storage.New(rootDir)
	if nil != err {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	var cleaned string

	cleaned, err = store.Clean("/escape")
	if nil != err {
		t.Fatalf("clean: %v", err)
	}

	// Opening through the escaping symlink must fail.
	var file, openErr = store.Open(cleaned)
	if nil == openErr {
		file.Close()
		t.Fatal("expected symlink escape to be blocked, but Open succeeded")
	}
}

func newTestStore(t *testing.T) *storage.Store {

	var store *storage.Store
	var err error

	store, _, err = storage.New(t.TempDir())
	if nil != err {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return store
}

// TestCleanNormalizes verifies safe paths are normalized and traversal is collapsed
// so the result can never escape the data root.
func TestCleanNormalizes(t *testing.T) {

	var store *storage.Store = newTestStore(t)

	var cases map[string]string = map[string]string{
		"/":                 "/",
		"":                  "/",
		"/a/b":              "/a/b",
		"a/b":               "/a/b",
		"/a/../b":           "/b",
		"/../../etc/passwd": "/etc/passwd",
		"\\a\\b":            "/a/b",
		"/a//b/":            "/a/b",
	}

	for raw, expected := range cases {

		var got string
		var err error

		got, err = store.Clean(raw)
		if nil != err {
			t.Errorf("Clean(%q) unexpected error: %v", raw, err)
			continue
		}

		if got != expected {
			t.Errorf("Clean(%q) = %q, want %q", raw, got, expected)
		}
	}
}

// TestFormatSize checks human-readable size rendering.
func TestFormatSize(t *testing.T) {

	var cases map[int64]string = map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1024:       "1.0 KB",
		1536:       "1.5 KB",
		1048576:    "1.0 MB",
		1073741824: "1.0 GB",
	}

	for size, expected := range cases {

		var got string = storage.FormatSize(size)
		if got != expected {
			t.Errorf("FormatSize(%d) = %q, want %q", size, got, expected)
		}
	}
}
