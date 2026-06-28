package web

import (
	"embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"tdrive/internal/config"
	"tdrive/internal/i18n"
	"tdrive/internal/storage"
	"tdrive/internal/view"
)

//go:embed static
var staticFiles embed.FS

// sessionTTL is how long a login session stays valid.
const sessionTTL time.Duration = 30 * 24 * time.Hour

// Server holds the dependencies for the HTTP surface and builds the handler chain.
type Server struct {
	config   *config.Config
	store    *storage.Store
	bundle   *i18n.Bundle
	sessions *sessionStore
	uploads  *uploadManager
}

// NewServer constructs the web server from its dependencies.
func NewServer(cfg *config.Config, store *storage.Store, bundle *i18n.Bundle) *Server {

	var server Server

	server.config = cfg
	server.store = store
	server.bundle = bundle
	server.sessions = newSessionStore(sessionTTL)
	server.uploads = newUploadManager(store)

	return &server
}

// Handler builds the fully-wrapped HTTP handler with routes and middleware.
func (server *Server) Handler() http.Handler {

	var mux *http.ServeMux = http.NewServeMux()

	// 1. Static assets (embedded for single-binary distribution).
	var assets fs.FS
	var err error

	assets, err = fs.Sub(staticFiles, "static")
	if nil == err {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets)))
	}

	// 2. Three access surfaces with distinct prefixes:
	//    /drive/{path}  the web UI (browse, view, edit)
	//    /raw/{path}    static access like nginx (raw bytes + directory autoindex)
	//    /api/v1/...    the REST API (registered below)
	mux.HandleFunc("GET /{$}", server.handleHome)
	mux.HandleFunc("GET /drive", server.handleHome)
	mux.HandleFunc("GET /drive/{path...}", server.handleBrowse)
	mux.HandleFunc("GET /view/{path...}", server.handleView)
	mux.HandleFunc("GET /edit/{path...}", server.handleEditForm)
	mux.HandleFunc("POST /edit/{path...}", server.handleEditSave)
	mux.HandleFunc("GET /raw/{path...}", server.handleRaw)
	mux.HandleFunc("GET /download/{path...}", server.handleDownload)

	// 3. Internal htmx/upload endpoints used by the web UI.
	mux.HandleFunc("GET /api/list", server.handleList)
	mux.HandleFunc("POST /api/mkdir", server.handleMkdir)
	mux.HandleFunc("POST /api/newfile", server.handleNewFile)
	mux.HandleFunc("POST /api/delete", server.handleDelete)
	mux.HandleFunc("POST /api/batch-delete", server.handleBatchDelete)
	mux.HandleFunc("POST /api/move", server.handleMove)
	mux.HandleFunc("GET /api/rename-form", server.handleRenameForm)
	mux.HandleFunc("POST /api/rename", server.handleRename)
	mux.HandleFunc("POST /api/upload/init", server.handleUploadInit)
	mux.HandleFunc("POST /api/upload/chunk", server.handleUploadChunk)
	mux.HandleFunc("POST /api/upload/complete", server.handleUploadComplete)
	mux.HandleFunc("POST /api/unzip", server.handleUnzip)
	mux.HandleFunc("POST /api/zip", server.handleZip)

	// 4. Public REST API (v1). The file resource lives at /api/v1/files/{path}:
	//    GET returns JSON (listing or metadata), PUT creates/replaces the file
	//    from the body, MKCOL creates a directory, PATCH moves/renames, DELETE
	//    removes. Raw bytes are downloaded from /api/v1/content/{path} (GET),
	//    keeping the JSON and binary representations on distinct URIs.
	//    Auth via session cookie or HTTP Basic Auth.
	mux.HandleFunc("GET /api/v1/files", server.handleApiFilesGet)
	mux.HandleFunc("GET /api/v1/files/{path...}", server.handleApiFilesGet)
	mux.HandleFunc("PUT /api/v1/files/{path...}", server.handleApiFilesPut)
	mux.HandleFunc("MKCOL /api/v1/files/{path...}", server.handleApiFilesMkcol)
	mux.HandleFunc("PATCH /api/v1/files/{path...}", server.handleApiFilesPatch)
	mux.HandleFunc("DELETE /api/v1/files/{path...}", server.handleApiFilesDelete)
	mux.HandleFunc("GET /api/v1/content/{path...}", server.handleApiContentGet)

	// 5. Auth and language.
	mux.HandleFunc("GET /login", server.handleLoginForm)
	mux.HandleFunc("POST /login", server.handleLoginSubmit)
	mux.HandleFunc("GET /logout", server.handleLogout)
	mux.HandleFunc("GET /lang/{code}", server.handleLanguage)

	// 6. WebDAV mount (serves the same files; authenticates via HTTP Basic Auth).
	var davHandler http.Handler = server.webdavHandler()

	mux.Handle("/webdav", davHandler)
	mux.Handle("/webdav/", davHandler)

	// 7. Health probe.
	mux.HandleFunc("GET /health", server.handleHealth)

	return server.wrap(mux)
}

// wrap applies the middleware chain in the documented order.
func (server *Server) wrap(handler http.Handler) http.Handler {

	handler = server.readOnlyMiddleware(handler)
	handler = server.authMiddleware(handler)
	handler = server.languageMiddleware(handler)
	handler = server.loggerMiddleware(handler)
	handler = server.requestIdMiddleware(handler)
	handler = server.recoverMiddleware(handler)

	return handler
}

// pageContext builds the shared template context for a request.
func (server *Server) pageContext(request *http.Request, title string) view.PageContext {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var page view.PageContext

	page.Title = title
	page.Lang = translator.Lang()
	page.AuthEnabled = server.config.AuthEnabled()
	page.CurrentURI = request.URL.RequestURI()

	return page
}

// render writes a templ component as an HTML response with the given status.
func (server *Server) render(writer http.ResponseWriter, request *http.Request, status int, component templ.Component) {

	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)

	var err error = component.Render(request.Context(), writer)
	if nil != err {
		// The header is already written; just log via the standard error path.
		http.Error(writer, "render error", http.StatusInternalServerError)
	}
}
