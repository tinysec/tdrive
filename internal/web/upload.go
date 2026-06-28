package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"tdrive/internal/storage"
)

// newUploadId returns a random 32-character hex id for an upload session.
func newUploadId() (string, error) {

	var raw []byte = make([]byte, 16)

	var _, err = rand.Read(raw)
	if nil != err {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

// uploadInitRequest is the JSON body the browser sends to start a chunked upload.
type uploadInitRequest struct {
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	Size        int64  `json:"size"`
	TotalChunks int    `json:"totalChunks"`
}

// uploadMeta is the persisted description of an in-progress upload session.
type uploadMeta struct {
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	Size        int64  `json:"size"`
	TotalChunks int    `json:"totalChunks"`
}

// uploadIdPattern restricts upload ids to the hex strings this server generates,
// preventing any path traversal through the id.
var uploadIdPattern *regexp.Regexp = regexp.MustCompile("^[a-f0-9]{32}$")

// uploadManager keeps in-progress chunked uploads in the OS temp directory (NOT
// in the served data root, so partial uploads never appear among the user's
// files). On completion the chunks are streamed into the final file via the
// store's atomic write.
type uploadManager struct {
	store    *storage.Store
	tempBase string
}

func newUploadManager(store *storage.Store) *uploadManager {

	var manager uploadManager

	manager.store = store
	manager.tempBase = storage.UploadTempBase()

	return &manager
}

func (manager *uploadManager) uploadDir(uploadId string) string {

	return filepath.Join(manager.tempBase, uploadId)
}

func (manager *uploadManager) chunkPath(uploadId string, index int) string {

	return filepath.Join(manager.tempBase, uploadId, fmt.Sprintf("%d.part", index))
}

func (manager *uploadManager) metaPath(uploadId string) string {

	return filepath.Join(manager.tempBase, uploadId, "meta.json")
}

// init validates the request, allocates an upload id, and persists the session metadata.
func (manager *uploadManager) init(request uploadInitRequest) (string, error) {

	// 1. Validate the target directory and file name.
	var dir string
	var err error

	dir, err = manager.store.Clean(request.Dir)
	if nil != err {
		return "", err
	}

	var name string

	name, err = sanitizeName(request.Name)
	if nil != err {
		return "", err
	}

	if request.TotalChunks < 1 {
		return "", fmt.Errorf("totalChunks must be at least 1")
	}

	// 2. Allocate an upload id and create its working directory in OS temp.
	var uploadId string

	uploadId, err = newUploadId()
	if nil != err {
		return "", err
	}

	err = os.MkdirAll(manager.uploadDir(uploadId), 0o700)
	if nil != err {
		return "", err
	}

	// 3. Persist the session metadata next to the chunks.
	var meta uploadMeta

	meta.Name = name
	meta.Dir = dir
	meta.Size = request.Size
	meta.TotalChunks = request.TotalChunks

	var raw []byte

	raw, err = json.Marshal(meta)
	if nil != err {
		return "", err
	}

	err = os.WriteFile(manager.metaPath(uploadId), raw, 0o600)
	if nil != err {
		return "", err
	}

	return uploadId, nil
}

// writeChunk stores one chunk's bytes by index.
func (manager *uploadManager) writeChunk(uploadId string, index int, body io.Reader) error {

	if false == uploadIdPattern.MatchString(uploadId) {
		return fmt.Errorf("invalid upload id")
	}

	if index < 0 {
		return fmt.Errorf("invalid chunk index")
	}

	var file *os.File
	var err error

	file, err = os.Create(manager.chunkPath(uploadId, index))
	if nil != err {
		return err
	}

	_, err = io.Copy(file, body)

	var closeErr error = file.Close()

	if nil != err {
		return err
	}

	return closeErr
}

// complete streams the chunks in order into the final file and removes the
// working directory. The final write is atomic (temp-then-rename in the
// destination directory) via the store.
func (manager *uploadManager) complete(uploadId string) (string, error) {

	if false == uploadIdPattern.MatchString(uploadId) {
		return "", fmt.Errorf("invalid upload id")
	}

	// 1. Load the session metadata and resolve the destination.
	var meta uploadMeta
	var err error

	meta, err = manager.readMeta(uploadId)
	if nil != err {
		return "", err
	}

	var target string

	target, err = manager.store.Clean(path.Join(meta.Dir, meta.Name))
	if nil != err {
		return "", err
	}

	// 2. Stream the chunks in order, opening only ONE at a time, and write the
	//    assembled result atomically into the data root. Opening every chunk at
	//    once would exhaust file descriptors for large (many-chunk) uploads.
	var reader *chunkSequenceReader = newChunkSequenceReader(manager, uploadId, meta.TotalChunks)

	_, err = manager.store.WriteFile(target, reader)

	_ = reader.Close()

	if nil != err {
		return "", err
	}

	_ = os.RemoveAll(manager.uploadDir(uploadId))

	return target, nil
}

// chunkSequenceReader concatenates an upload's chunk files into one stream,
// opening each chunk only when the previous one is exhausted. This keeps exactly
// one chunk file open at a time, so the file-descriptor cost is constant no
// matter how many chunks a large upload has.
type chunkSequenceReader struct {
	manager     *uploadManager
	uploadId    string
	totalChunks int
	nextIndex   int
	current     *os.File
}

func newChunkSequenceReader(manager *uploadManager, uploadId string, totalChunks int) *chunkSequenceReader {

	var reader chunkSequenceReader

	reader.manager = manager
	reader.uploadId = uploadId
	reader.totalChunks = totalChunks

	return &reader
}

func (reader *chunkSequenceReader) Read(buffer []byte) (int, error) {

	for {
		// Open the next chunk when no chunk is currently open.
		if nil == reader.current {

			if reader.nextIndex >= reader.totalChunks {
				return 0, io.EOF
			}

			var file *os.File
			var err error

			file, err = os.Open(reader.manager.chunkPath(reader.uploadId, reader.nextIndex))
			if nil != err {
				return 0, fmt.Errorf("missing chunk %d: %w", reader.nextIndex, err)
			}

			reader.current = file
			reader.nextIndex = reader.nextIndex + 1
		}

		var read int
		var err error

		read, err = reader.current.Read(buffer)

		// At the end of this chunk, close it and continue with the next one.
		if io.EOF == err {

			_ = reader.current.Close()
			reader.current = nil

			if read > 0 {
				return read, nil
			}

			continue
		}

		return read, err
	}
}

func (reader *chunkSequenceReader) Close() error {

	if nil != reader.current {

		var err error = reader.current.Close()

		reader.current = nil

		return err
	}

	return nil
}

func (manager *uploadManager) readMeta(uploadId string) (uploadMeta, error) {

	var meta uploadMeta

	var raw []byte
	var err error

	raw, err = os.ReadFile(manager.metaPath(uploadId))
	if nil != err {
		return meta, err
	}

	err = json.Unmarshal(raw, &meta)
	if nil != err {
		return meta, err
	}

	return meta, nil
}

// sanitizeName reduces a client-supplied filename to a safe base name.
func sanitizeName(raw string) (string, error) {

	var normalized string = strings.ReplaceAll(raw, "\\", "/")

	var base string = path.Base(normalized)

	base = strings.TrimSpace(base)

	if "" == base || "." == base || ".." == base || "/" == base {
		return "", fmt.Errorf("invalid file name: %q", raw)
	}

	return base, nil
}
