package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"tdrive/internal/i18n"
)

// unzipRequest is the JSON body for /api/unzip.
type unzipRequest struct {
	Path string `json:"path"` // the .zip file to extract
	Dir  string `json:"dir"`  // destination directory; defaults to the zip's parent
}

// handleUnzip extracts a ZIP archive into a directory. The destination defaults
// to the archive's containing directory and is always cleaned through the store,
// so the per-entry containment check in extractZip is the final authority on
// where bytes may land.
func (server *Server) handleUnzip(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var body unzipRequest
	if nil != json.NewDecoder(request.Body).Decode(&body) {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}

	zipPath, err := server.store.Clean(body.Path)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var destDir string
	if "" != strings.TrimSpace(body.Dir) {
		destDir, err = server.store.Clean(body.Dir)
		if nil != err {
			http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
			return
		}
	} else {
		destDir = path.Dir(zipPath)
	}

	if extractErr := extractZip(server.store, zipPath, destDir); nil != extractErr {
		slog.Warn("unzip failed", slog.String("path", zipPath), slog.String("error", extractErr.Error()))
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	server.renderListPartial(writer, request, destDir)
}

// zipRequest is the JSON body for /api/zip.
type zipRequest struct {
	Paths []string `json:"paths"` // files/directories to pack
	Dest  string   `json:"dest"`  // destination .zip path (relative to a dir)
	Dir   string   `json:"dir"`   // directory to refresh afterwards
}

// handleZip packs one or more files/directories into a new ZIP. Dest is resolved
// under the refresh directory when relative, and each source is validated
// through the store. The result is written atomically, then the directory is
// re-listed.
func (server *Server) handleZip(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var body zipRequest
	if nil != json.NewDecoder(request.Body).Decode(&body) {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}

	dir, err := server.store.Clean(body.Dir)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	if 0 == len(body.Paths) {
		http.Error(writer, translator.T("error.generic"), http.StatusBadRequest)
		return
	}

	// Resolve sources relative to dir when they are not absolute.
	sources := make([]string, 0, len(body.Paths))
	for _, raw := range body.Paths {
		var src string
		if strings.HasPrefix(raw, "/") {
			src, err = server.store.Clean(raw)
		} else {
			src, err = server.store.Clean(path.Join(dir, raw))
		}
		if nil != err {
			http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
			return
		}
		sources = append(sources, src)
	}

	dest, err := server.store.Clean(path.Join(dir, body.Dest))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	if createErr := createZip(server.store, sources, dest); nil != createErr {
		slog.Warn("zip failed", slog.String("dest", dest), slog.String("error", createErr.Error()))
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	server.renderListPartial(writer, request, dir)
}