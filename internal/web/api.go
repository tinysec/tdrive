package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/spf13/afero"

	"tdrive/internal/storage"
)

// The REST API uses two resource trees:
//
//	/api/v1/files/{path}     JSON metadata and directory structure
//	/api/v1/content/{path}   the raw bytes of a file
//
// Metadata and bytes are never returned from the same URI, so each URI has a
// single representation. Standard HTTP methods carry all operations; no verbs
// appear in the path.

// apiEntry is the JSON representation of a file or directory.
type apiEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Type    string `json:"type"` // "file" or "dir"
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`

	// ContentUrl points to the bytes of a file; omitted for directories.
	ContentUrl string `json:"contentUrl,omitempty"`
}

// apiListResponse is returned when reading a directory.
type apiListResponse struct {
	Path    string     `json:"path"`
	Type    string     `json:"type"` // always "dir"
	Entries []apiEntry `json:"entries"`
}

// apiPatchRequest is the body of PATCH /api/v1/files/{path}: it relocates the
// resource (rename/move) to a new path.
type apiPatchRequest struct {
	Path string `json:"path"`
}

// handleApiFilesGet serves GET /api/v1/files/{path...}: it always returns JSON —
// a directory's listing or a file's metadata. File bytes live under /content.
func (server *Server) handleApiFilesGet(writer http.ResponseWriter, request *http.Request) {

	var cleaned string
	var err error

	cleaned, err = server.store.Clean(apiPath(request))
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden path")
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr {
		writeApiError(writer, http.StatusNotFound, "not found")
		return
	}

	if info.IsDir() {
		server.apiListDir(writer, request, cleaned)
		return
	}

	writeApiJson(writer, http.StatusOK, toApiEntry(cleaned, info))
}

// apiListDir writes the JSON listing of a directory.
func (server *Server) apiListDir(writer http.ResponseWriter, request *http.Request, cleaned string) {

	var entries []storage.Entry
	var err error

	entries, err = server.store.List(cleaned)
	if nil != err {
		writeApiError(writer, http.StatusInternalServerError, "cannot list directory")
		return
	}

	var response apiListResponse

	response.Path = cleaned
	response.Type = "dir"
	response.Entries = make([]apiEntry, 0, len(entries))

	for _, entry := range entries {

		var child string = path.Join(cleaned, entry.Name)

		var item apiEntry

		item.Name = entry.Name
		item.Path = child
		item.Type = entryType(entry.IsDir)
		item.Size = entry.Size
		item.ModTime = entry.ModTime.UTC().Format(time.RFC3339)

		if false == entry.IsDir {
			item.ContentUrl = apiContentUrl(child)
		}

		response.Entries = append(response.Entries, item)
	}

	writeApiJson(writer, http.StatusOK, response)
}

// handleApiFilesPatch serves PATCH /api/v1/files/{path...}: it relocates the
// resource to the path given in the body (rename/move).
func (server *Server) handleApiFilesPatch(writer http.ResponseWriter, request *http.Request) {

	var from string
	var err error

	from, err = server.store.Clean(apiPath(request))
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden path")
		return
	}

	var body apiPatchRequest

	err = json.NewDecoder(request.Body).Decode(&body)
	if nil != err {
		writeApiError(writer, http.StatusBadRequest, "invalid request body")
		return
	}

	var to string

	to, err = server.store.Clean(body.Path)
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden target path")
		return
	}

	_, err = server.store.Stat(from)
	if nil != err {
		writeApiError(writer, http.StatusNotFound, "not found")
		return
	}

	err = server.store.Rename(from, to)
	if nil != err {
		writeApiError(writer, http.StatusInternalServerError, "move failed")
		return
	}

	server.writeEntry(writer, http.StatusOK, to)
}

// handleApiFilesDelete serves DELETE /api/v1/files/{path...}.
func (server *Server) handleApiFilesDelete(writer http.ResponseWriter, request *http.Request) {

	var cleaned string
	var err error

	cleaned, err = server.store.Clean(apiPath(request))
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden path")
		return
	}

	if "/" == cleaned {
		writeApiError(writer, http.StatusBadRequest, "cannot delete root")
		return
	}

	_, err = server.store.Stat(cleaned)
	if nil != err {
		writeApiError(writer, http.StatusNotFound, "not found")
		return
	}

	err = server.store.Remove(cleaned)
	if nil != err {
		writeApiError(writer, http.StatusInternalServerError, "delete failed")
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

// handleApiContentGet serves GET /api/v1/content/{path...}: it streams a file's
// bytes with Range support.
func (server *Server) handleApiContentGet(writer http.ResponseWriter, request *http.Request) {

	var cleaned string
	var err error

	cleaned, err = server.store.Clean(apiPath(request))
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden path")
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr {
		writeApiError(writer, http.StatusNotFound, "not found")
		return
	}

	if info.IsDir() {
		writeApiError(writer, http.StatusBadRequest, "path is a directory")
		return
	}

	var file afero.File

	file, err = server.store.Open(cleaned)
	if nil != err {
		writeApiError(writer, http.StatusNotFound, "not found")
		return
	}

	defer file.Close()

	http.ServeContent(writer, request, info.Name(), info.ModTime(), file)
}

// handleApiFilesPut serves PUT /api/v1/files/{path...}: it creates or replaces
// the file at this URI from the request body, creating any missing parent
// directories. This is the standard REST idiom — PUT a representation to the
// resource's own URI.
func (server *Server) handleApiFilesPut(writer http.ResponseWriter, request *http.Request) {

	var cleaned string
	var err error

	cleaned, err = server.store.Clean(apiPath(request))
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden path")
		return
	}

	if "/" == cleaned {
		writeApiError(writer, http.StatusBadRequest, "cannot write to root")
		return
	}

	_, err = server.store.WriteFile(cleaned, request.Body)
	if nil != err {
		writeApiError(writer, http.StatusInternalServerError, "write failed")
		return
	}

	server.writeEntry(writer, http.StatusCreated, cleaned)
}

// handleApiFilesMkcol serves MKCOL /api/v1/files/{path...}: it creates a
// directory at this URI. MKCOL (RFC 4918) is the standard HTTP method for
// creating a collection, the same one the WebDAV surface uses.
func (server *Server) handleApiFilesMkcol(writer http.ResponseWriter, request *http.Request) {

	var cleaned string
	var err error

	cleaned, err = server.store.Clean(apiPath(request))
	if nil != err {
		writeApiError(writer, http.StatusForbidden, "forbidden path")
		return
	}

	if "/" == cleaned {
		writeApiError(writer, http.StatusBadRequest, "root already exists")
		return
	}

	err = server.store.CreateDir(cleaned)
	if nil != err {
		writeApiError(writer, http.StatusInternalServerError, "mkdir failed")
		return
	}

	server.writeEntry(writer, http.StatusCreated, cleaned)
}

// writeEntry stats a path and writes its JSON metadata representation.
func (server *Server) writeEntry(writer http.ResponseWriter, status int, cleaned string) {

	var info, err = server.store.Stat(cleaned)
	if nil != err {
		writeApiError(writer, http.StatusInternalServerError, "cannot stat result")
		return
	}

	writeApiJson(writer, status, toApiEntry(cleaned, info))
}

// apiPath extracts the slash-rooted path from a {path...} route.
func apiPath(request *http.Request) string {

	return "/" + request.PathValue("path")
}

// apiContentUrl builds the bytes URL for a file path.
func apiContentUrl(cleaned string) string {

	var target url.URL

	target.Path = "/api/v1/content" + cleaned

	return target.String()
}

// entryType maps a directory flag to the API type string.
func entryType(isDir bool) string {

	if isDir {
		return "dir"
	}

	return "file"
}

// toApiEntry builds the metadata representation of one filesystem entry.
func toApiEntry(cleaned string, info os.FileInfo) apiEntry {

	var entry apiEntry

	entry.Name = info.Name()
	entry.Path = cleaned
	entry.Type = entryType(info.IsDir())
	entry.Size = info.Size()
	entry.ModTime = info.ModTime().UTC().Format(time.RFC3339)

	if false == info.IsDir() {
		entry.ContentUrl = apiContentUrl(cleaned)
	}

	return entry
}

// writeApiJson writes v as a JSON response with the given status.
func writeApiJson(writer http.ResponseWriter, status int, value any) {

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)

	_ = json.NewEncoder(writer).Encode(value)
}

// writeApiError writes a JSON error response.
func writeApiError(writer http.ResponseWriter, status int, message string) {

	writeApiJson(writer, status, map[string]string{"error": message})
}
