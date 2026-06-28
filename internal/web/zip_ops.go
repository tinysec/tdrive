package web

import (
	"archive/zip"
	"errors"
	"io"
	"path"
	"strings"

	"tdrive/internal/storage"
)

// zipEntry pairs an archive-relative name with its source path on the store.
type zipEntry struct {
	zipName string
	fsPath  string
	isDir   bool
}

// extractZip unpacks the ZIP archive at zipPath into destDir, recreating the
// stored directory structure. It is hardened against zip-slip: every entry name
// is resolved against destDir and rejected if it would escape that directory, so
// a malicious archive cannot write outside the target tree. Memory use is
// bounded because each entry is streamed straight to the store.
func extractZip(store *storage.Store, zipPath string, destDir string) error {

	reader, openErr := store.Open(zipPath)
	if nil != openErr {
		return openErr
	}
	defer reader.Close()

	info, statErr := store.Stat(zipPath)
	if nil != statErr {
		return statErr
	}

	zipReader, zipErr := zip.NewReader(reader, info.Size())
	if nil != zipErr {
		return zipErr
	}

	destDir = path.Clean(destDir)

	for _, entry := range zipReader.File {

		name := strings.ReplaceAll(entry.Name, "\\", "/")
		name = strings.TrimLeft(name, "/")

		if "" == name {
			continue
		}

		// path.Join resolves "..", then store.Clean re-validates; the containment
		// check below rejects anything that still escapes destDir.
		cleaned, cleanErr := store.Clean(path.Join(destDir, name))
		if nil != cleanErr {
			return cleanErr
		}

		if false == isWithin(cleaned, destDir) {
			return errors.New("zip entry escapes destination: " + entry.Name)
		}

		if entry.FileInfo().IsDir() {
			if mkErr := store.CreateDir(cleaned); nil != mkErr {
				return mkErr
			}
			continue
		}

		entryReader, openEntryErr := entry.Open()
		if nil != openEntryErr {
			return openEntryErr
		}

		_, writeErr := store.WriteFile(cleaned, entryReader)
		entryReader.Close()
		if nil != writeErr {
			return writeErr
		}
	}

	return nil
}

// isWithin reports whether target is equal to or nested inside base (both in
// cleaned "/..." form). "/a/b" is within "/a"; "/aa" is not within "/a".
func isWithin(target string, base string) bool {

	if base == "/" {
		return true
	}

	if target == base {
		return true
	}

	return strings.HasPrefix(target, base+"/")
}

// createZip packs the given paths into a new ZIP archive written at dest. Each
// source may be a file or a directory; directories are walked recursively. The
// archive root contains each source's base name. The archive is streamed through
// a pipe into store.WriteFile, so it lands atomically and a partial archive
// never appears among the user's files.
func createZip(store *storage.Store, sources []string, dest string) error {

	var entries []zipEntry

	for _, src := range sources {

		info, statErr := store.Stat(src)
		if nil != statErr {
			return statErr
		}

		baseName := path.Base(src)

		if false == info.IsDir() {
			entries = append(entries, zipEntry{zipName: baseName, fsPath: src})
			continue
		}

		collected, walkErr := collectZipDir(store, src, baseName)
		if nil != walkErr {
			return walkErr
		}
		entries = append(entries, collected...)
	}

	pipeReader, pipeWriter := io.Pipe()

	writeDone := make(chan error, 1)
	go func() {
		_, err := store.WriteFile(dest, pipeReader)
		writeDone <- err
		pipeReader.Close()
	}()

	zipWriter := zip.NewWriter(pipeWriter)
	writeErr := writeZipEntries(store, entries, zipWriter, pipeWriter)

	<-writeDone

	return writeErr
}

// writeZipEntries serializes the collected entries into zipWriter. It owns the
// zip writer's lifecycle (Close) and signals any failure to the pipe so the
// WriteFile goroutine unblocks.
func writeZipEntries(store *storage.Store, entries []zipEntry, zipWriter *zip.Writer, pipeWriter *io.PipeWriter) error {

	for _, e := range entries {
		if e.isDir {
			if _, err := zipWriter.Create(e.zipName + "/"); nil != err {
				return failZip(zipWriter, pipeWriter, err)
			}
			continue
		}

		header := &zip.FileHeader{Name: e.zipName, Method: zip.Deflate}
		fileWriter, createErr := zipWriter.CreateHeader(header)
		if nil != createErr {
			return failZip(zipWriter, pipeWriter, createErr)
		}

		file, openErr := store.Open(e.fsPath)
		if nil != openErr {
			return failZip(zipWriter, pipeWriter, openErr)
		}

		_, copyErr := io.Copy(fileWriter, file)
		file.Close()
		if nil != copyErr {
			return failZip(zipWriter, pipeWriter, copyErr)
		}
	}

	closeErr := zipWriter.Close()
	pipeWriter.Close()
	return closeErr
}

// failZip closes the writer and pipe on an error so the WriteFile goroutine
// terminates instead of deadlocking.
func failZip(zipWriter *zip.Writer, pipeWriter *io.PipeWriter, err error) error {
	zipWriter.Close()
	pipeWriter.CloseWithError(err)
	return err
}

// collectZipDir walks a directory tree one level at a time via store.List and
// returns archive entries rooted at rootName. Empty directories are included so
// the structure is preserved.
func collectZipDir(store *storage.Store, cleaned string, rootName string) ([]zipEntry, error) {

	var result []zipEntry

	type pending struct {
		fsPath  string
		zipName string
	}

	stack := []pending{{fsPath: cleaned, zipName: rootName}}

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		result = append(result, zipEntry{zipName: current.zipName, fsPath: current.fsPath, isDir: true})

		entries, listErr := store.List(current.fsPath)
		if nil != listErr {
			return nil, listErr
		}

		for _, e := range entries {
			childFs := path.Join(current.fsPath, e.Name)
			childZip := current.zipName + "/" + e.Name

			if e.IsDir {
				stack = append(stack, pending{fsPath: childFs, zipName: childZip})
			} else {
				result = append(result, zipEntry{zipName: childZip, fsPath: childFs})
			}
		}
	}

	return result, nil
}