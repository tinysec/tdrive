package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tdrive/internal/meta"

	"github.com/spf13/afero"
)

// Store is a path-safe view over a single data directory. The same underlying
// afero filesystem is shared by the HTTP and FTP servers so both surfaces see
// identical files and are sandboxed to the data root.
type Store struct {
	fs afero.Fs
}

// Entry describes one item in a directory listing.
type Entry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// New creates a Store rooted at the given OS directory, creating it if missing.
// It returns the Store and the shared afero filesystem for the FTP driver to reuse.
func New(root string) (*Store, afero.Fs, error) {

	// 1. Ensure the data root exists.
	var err error = os.MkdirAll(root, 0o755)
	if nil != err {
		return nil, nil, fmt.Errorf("create data root %q: %w", root, err)
	}

	// 2. Open an OS-enforced root. Every operation is confined beneath this
	//    directory by the kernel, and symlinks cannot escape it — stronger than
	//    lexical path cleaning. The same sandbox is shared by HTTP and FTP.
	var osRoot *os.Root

	osRoot, err = os.OpenRoot(root)
	if nil != err {
		return nil, nil, fmt.Errorf("open data root %q: %w", root, err)
	}

	var base afero.Fs = newRootFs(osRoot)

	var store Store

	store.fs = base

	return &store, base, nil
}

// Fs exposes the shared sandboxed filesystem (used by the FTP driver).
func (store *Store) Fs() afero.Fs {

	return store.fs
}

// Close releases the underlying OS root handle.
func (store *Store) Close() error {

	type closer interface {
		Close() error
	}

	var fsCloser closer
	var ok bool

	fsCloser, ok = store.fs.(closer)
	if false == ok {
		return nil
	}

	return fsCloser.Close()
}

// Clean validates and normalizes a user-supplied path into a canonical
// slash-rooted path (e.g. "/a/b"). It rejects any path that would escape the
// data root. (The os.Root filesystem enforces containment at the OS level too.)
func (store *Store) Clean(raw string) (string, error) {

	// 1. Normalize separators and resolve "." / ".." against an absolute root.
	var normalized string = strings.ReplaceAll(raw, "\\", "/")

	if false == strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}

	var cleaned string = path.Clean(normalized)

	// 2. After cleaning against "/", no ".." can remain — but guard explicitly.
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("invalid path: %q", raw)
	}

	return cleaned, nil
}

// List returns the entries of a directory.
func (store *Store) List(dir string) ([]Entry, error) {

	var infos []os.FileInfo
	var err error

	infos, err = afero.ReadDir(store.fs, dir)
	if nil != err {
		return nil, err
	}

	var entries []Entry = make([]Entry, 0, len(infos))

	for _, info := range infos {

		var entry Entry

		entry.Name = info.Name()
		entry.IsDir = info.IsDir()
		entry.Size = info.Size()
		entry.ModTime = info.ModTime()

		entries = append(entries, entry)
	}

	sortEntries(entries)

	return entries, nil
}

// Stat returns file information for a cleaned path.
func (store *Store) Stat(cleaned string) (os.FileInfo, error) {

	return store.fs.Stat(cleaned)
}

// Open opens a file for reading. The returned file satisfies io.ReadSeeker so it
// works directly with http.ServeContent for ranged, streaming downloads.
func (store *Store) Open(cleaned string) (afero.File, error) {

	return store.fs.Open(cleaned)
}

// CreateDir creates a directory and any missing parents.
func (store *Store) CreateDir(cleaned string) error {

	return store.fs.MkdirAll(cleaned, 0o755)
}

// Remove deletes a file or directory (recursively for directories).
func (store *Store) Remove(cleaned string) error {

	return store.fs.RemoveAll(cleaned)
}

// Rename moves a file or directory within the data root.
func (store *Store) Rename(oldCleaned string, newCleaned string) error {

	return store.fs.Rename(oldCleaned, newCleaned)
}

// WriteFile streams src into the file at cleaned, creating parent directories.
// It writes to a temporary file IN THE DESTINATION DIRECTORY (guaranteed same
// filesystem) and atomically renames it into place, so a partial write never
// leaves a corrupt file — and no central control directory is needed.
func (store *Store) WriteFile(cleaned string, src io.Reader) (int64, error) {

	// 1. Ensure the parent directory exists.
	var parent string = path.Dir(cleaned)

	var err error = store.fs.MkdirAll(parent, 0o755)
	if nil != err {
		return 0, err
	}

	// 2. Stream into a hidden temp file next to the destination.
	var suffix string

	suffix, err = randomHex()
	if nil != err {
		return 0, err
	}

	var tempPath string = path.Join(parent, "."+suffix+".tmp")

	var file afero.File

	file, err = store.fs.Create(tempPath)
	if nil != err {
		return 0, err
	}

	var written int64

	written, err = io.Copy(file, src)

	var closeErr error = file.Close()

	if nil != err {
		_ = store.fs.Remove(tempPath)
		return 0, err
	}

	if nil != closeErr {
		_ = store.fs.Remove(tempPath)
		return 0, closeErr
	}

	// 3. Atomically move the temp file into place.
	err = store.fs.Rename(tempPath, cleaned)
	if nil != err {
		_ = store.fs.Remove(tempPath)
		return 0, err
	}

	return written, nil
}

// randomHex returns a 32-character random hex string for temp file names.
func randomHex() (string, error) {

	var raw []byte = make([]byte, 16)

	var _, err = rand.Read(raw)
	if nil != err {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

// UploadTempBase is the OS-temp directory that holds in-progress chunked uploads.
// It lives outside the served data root so partial uploads never appear among the
// user's files.
func UploadTempBase() string {

	return filepath.Join(os.TempDir(), meta.Name+"-uploads")
}

// CleanupUploadTemp removes abandoned upload working directories older than maxAge.
// Incomplete uploads would otherwise accumulate, since no database tracks them.
func CleanupUploadTemp(maxAge time.Duration) error {

	var base string = UploadTempBase()

	var entries []os.DirEntry
	var err error

	entries, err = os.ReadDir(base)
	if nil != err {
		// A missing temp directory is not an error worth surfacing.
		return nil
	}

	var cutoff time.Time = time.Now().Add(-maxAge)

	for _, entry := range entries {

		var info os.FileInfo

		info, err = entry.Info()
		if nil != err {
			continue
		}

		if info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(base, entry.Name()))
		}
	}

	return nil
}

// FormatSize renders a byte count as a human-readable string.
func FormatSize(size int64) string {

	const unit int64 = 1024

	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	var div int64 = unit
	var exp int = 0

	var labels []string = []string{"KB", "MB", "GB", "TB", "PB"}

	for n := size / unit; n >= unit && exp < len(labels)-1; n = n / unit {

		div = div * unit
		exp = exp + 1
	}

	var value float64 = float64(size) / float64(div)

	return fmt.Sprintf("%.1f %s", value, labels[exp])
}

// sortEntries orders directories first, then by name (case-insensitive).
func sortEntries(entries []Entry) {

	sort.Slice(entries, func(i int, j int) bool {

		var a Entry = entries[i]
		var b Entry = entries[j]

		if a.IsDir != b.IsDir {
			return a.IsDir
		}

		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}
