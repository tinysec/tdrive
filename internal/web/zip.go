package web

import (
	"archive/zip"
	"io"
	"path"

	"tdrive/internal/storage"
)

// streamDirZip walks the directory at cleaned and writes its contents to the
// client as a single ZIP archive, streamed so memory use stays bounded no matter
// how large the tree is. The archive's root folder is named after the directory,
// so downloading "/a/b" yields entries like "b/...". The walk is driven by
// store.List (one level at a time) rather than afero.Walk, because the sandboxed
// root filesystem does not implement the Walk contract uniformly.
func streamDirZip(store *storage.Store, cleaned string, writer io.Writer) error {

	zipWriter := zip.NewWriter(writer)

	rootName := path.Base(cleaned)
	if "/" == cleaned {
		rootName = "tdrive"
	}

	type pending struct {
		fsPath  string
		zipName string
	}

	stack := []pending{{fsPath: cleaned, zipName: rootName}}

	for len(stack) > 0 {

		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Create the directory entry first so empty folders are preserved too.
		if _, dirErr := zipWriter.Create(current.zipName + "/"); nil != dirErr {
			zipWriter.Close()
			return dirErr
		}

		entries, listErr := store.List(current.fsPath)
		if nil != listErr {
			zipWriter.Close()
			return listErr
		}

		for _, entry := range entries {

			childFs := path.Join(current.fsPath, entry.Name)
			childZip := current.zipName + "/" + entry.Name

			if entry.IsDir {
				stack = append(stack, pending{fsPath: childFs, zipName: childZip})
				continue
			}

			header := &zip.FileHeader{Name: childZip, Method: zip.Deflate}
			fileWriter, createErr := zipWriter.CreateHeader(header)
			if nil != createErr {
				zipWriter.Close()
				return createErr
			}

			file, openErr := store.Open(childFs)
			if nil != openErr {
				zipWriter.Close()
				return openErr
			}

			_, copyErr := io.Copy(fileWriter, file)
			file.Close()
			if nil != copyErr {
				zipWriter.Close()
				return copyErr
			}
		}
	}

	return zipWriter.Close()
}