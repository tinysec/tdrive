package web

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"tdrive/internal/storage"
)

// newTestUploadManager builds an upload manager backed by a temp data root, with
// its chunk staging directory isolated to the test's temp dir.
func newTestUploadManager(t *testing.T) (*uploadManager, *storage.Store) {

	t.Helper()

	var store *storage.Store
	var err error

	store, _, err = storage.New(t.TempDir())
	if nil != err {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	var manager *uploadManager = newUploadManager(store)

	manager.tempBase = filepath.Join(t.TempDir(), "uploads")

	return manager, store
}

// TestChunkedUploadManyChunks assembles an upload from many chunks (including an
// empty one) and checks the result is the exact concatenation. This is the
// regression test for the file-descriptor fix: complete() must stream the chunks
// one at a time, not open them all at once.
func TestChunkedUploadManyChunks(t *testing.T) {

	var manager, store = newTestUploadManager(t)

	const totalChunks int = 64

	var expected bytes.Buffer

	var uploadId string
	var err error

	uploadId, err = manager.init(uploadInitRequest{Name: "big.bin", Dir: "/", Size: 0, TotalChunks: totalChunks})
	if nil != err {
		t.Fatalf("init: %v", err)
	}

	for index := 0; index < totalChunks; index = index + 1 {

		var chunk []byte

		// Leave chunk 7 empty to exercise the zero-byte path in the reader.
		if 7 != index {
			chunk = []byte(fmt.Sprintf("chunk-%03d-payload;", index))
		}

		expected.Write(chunk)

		err = manager.writeChunk(uploadId, index, bytes.NewReader(chunk))
		if nil != err {
			t.Fatalf("writeChunk %d: %v", index, err)
		}
	}

	var target string

	target, err = manager.complete(uploadId)
	if nil != err {
		t.Fatalf("complete: %v", err)
	}

	if "/big.bin" != target {
		t.Errorf("target = %q, want /big.bin", target)
	}

	var assembled []byte = readStored(t, store, target)

	if false == bytes.Equal(assembled, expected.Bytes()) {
		t.Errorf("assembled file does not match the concatenated chunks (len %d vs %d)", len(assembled), expected.Len())
	}
}

// TestChunkedUploadMissingChunk verifies completion fails clearly when a chunk is
// absent, rather than producing a truncated file.
func TestChunkedUploadMissingChunk(t *testing.T) {

	var manager, _ = newTestUploadManager(t)

	var uploadId string
	var err error

	uploadId, err = manager.init(uploadInitRequest{Name: "x.bin", Dir: "/", Size: 0, TotalChunks: 3})
	if nil != err {
		t.Fatalf("init: %v", err)
	}

	// Write chunks 0 and 2 but not 1.
	_ = manager.writeChunk(uploadId, 0, bytes.NewReader([]byte("a")))
	_ = manager.writeChunk(uploadId, 2, bytes.NewReader([]byte("c")))

	_, err = manager.complete(uploadId)
	if nil == err {
		t.Errorf("complete should fail when a chunk is missing")
	}
}

// readStored reads a stored file's full contents through the store.
func readStored(t *testing.T, store *storage.Store, cleaned string) []byte {

	t.Helper()

	var file io.ReadCloser
	var err error

	file, err = store.Open(cleaned)
	if nil != err {
		t.Fatalf("open %q: %v", cleaned, err)
	}

	defer file.Close()

	var data []byte

	data, err = io.ReadAll(file)
	if nil != err {
		t.Fatalf("read %q: %v", cleaned, err)
	}

	return data
}
