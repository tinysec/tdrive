package web

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/afero"

	"tdrive/internal/i18n"
	"tdrive/internal/meta"
	"tdrive/internal/storage"
	"tdrive/internal/view"
)

// modTimeLayout is the human-readable timestamp format used in listings.
const modTimeLayout string = "2006-01-02 15:04"

// rawIndexNameWidth is the column the /raw autoindex pads names to before the
// modification time and size (nginx-style alignment).
const rawIndexNameWidth int = 50

// handleHealth is the load-balancer / container readiness probe.
func (server *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {

	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("ok"))
}

// handleHome redirects the site root to the web UI at /drive/.
func (server *Server) handleHome(writer http.ResponseWriter, request *http.Request) {

	http.Redirect(writer, request, "/drive/", http.StatusFound)
}

// handleBrowse serves the web UI directory listing under /drive.
func (server *Server) handleBrowse(writer http.ResponseWriter, request *http.Request) {

	server.browse(writer, request, "/"+request.PathValue("path"))
}

// browse renders the full directory page, or redirects to the viewer for a file.
func (server *Server) browse(writer http.ResponseWriter, request *http.Request, rawPath string) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	// 1. Validate and resolve the requested path.
	var cleaned string
	var err error

	cleaned, err = server.store.Clean(rawPath)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	// 2. A file path redirects to its preview page.
	if false == info.IsDir() {
		http.Redirect(writer, request, view.ViewURL(cleaned), http.StatusFound)
		return
	}

	// 3. Build and render the directory page.
	var data view.BrowseData

	data, err = server.buildBrowseData(request, cleaned)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	data.Page = server.pageContext(request, meta.Name)

	server.render(writer, request, http.StatusOK, view.BrowsePage(data))
}

// handleList returns the file-list partial for htmx swaps after mutations/uploads.
func (server *Server) handleList(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var cleaned string
	var err error

	cleaned, err = server.store.Clean(request.URL.Query().Get("dir"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var data view.BrowseData

	data, err = server.buildBrowseData(request, cleaned)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	server.render(writer, request, http.StatusOK, view.FileList(data))
}

// handleMkdir creates a sub-directory and returns the refreshed listing.
func (server *Server) handleMkdir(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var dir string = request.FormValue("dir")
	var name string = request.FormValue("name")

	// 1. Resolve the parent directory and validate the new folder name.
	var parent string
	var err error

	parent, err = server.store.Clean(dir)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var base string

	base, err = sanitizeName(name)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusBadRequest)
		return
	}

	// 2. Create the directory.
	var target string

	target, err = server.store.Clean(path.Join(parent, base))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	err = server.store.CreateDir(target)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	// 3. Return the refreshed listing of the parent directory.
	server.renderListPartial(writer, request, parent)
}

// handleNewFile creates an empty text file in the given directory and asks the
// browser (via htmx's HX-Redirect) to open it in the editor. It refuses to
// overwrite an existing entry.
func (server *Server) handleNewFile(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var dir string = request.FormValue("dir")
	var name string = request.FormValue("name")

	// 1. Resolve the parent directory and validate the new file name.
	var parent string
	var err error

	parent, err = server.store.Clean(dir)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var base string

	base, err = sanitizeName(name)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusBadRequest)
		return
	}

	var target string

	target, err = server.store.Clean(path.Join(parent, base))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	// 2. Refuse to clobber an existing file or directory.
	var _, statErr = server.store.Stat(target)
	if nil == statErr {
		http.Error(writer, translator.T("error.exists"), http.StatusConflict)
		return
	}

	// 3. Create the empty file.
	_, err = server.store.WriteFile(target, strings.NewReader(""))
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	// 4. Ask htmx to open the new file in the editor.
	writer.Header().Set("HX-Redirect", view.EditURL(target))
	writer.WriteHeader(http.StatusNoContent)
}

// handleDelete removes a file or directory and returns the refreshed listing.
func (server *Server) handleDelete(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var targetRaw string = request.FormValue("path")
	var dir string = request.FormValue("dir")

	// 1. Validate both the target and the directory to refresh.
	var target string
	var err error

	target, err = server.store.Clean(targetRaw)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var parent string

	parent, err = server.store.Clean(dir)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	// 2. Remove the target.
	err = server.store.Remove(target)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	// 3. Return the refreshed listing.
	server.renderListPartial(writer, request, parent)
}

// handleMove moves a dragged entry into a target directory (drag-and-drop),
// then returns the refreshed listing of the current directory.
func (server *Server) handleMove(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	// 1. Resolve the source, the directory to refresh, and the target directory.
	var source string
	var err error

	source, err = server.store.Clean(request.FormValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var current string

	current, err = server.store.Clean(request.FormValue("dir"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var targetDir string

	targetDir, err = server.store.Clean(request.FormValue("targetDir"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	// 2. Build the destination and move, unless it is a no-op or would move a
	//    directory into itself or a descendant.
	var dest string

	dest, err = server.store.Clean(path.Join(targetDir, path.Base(source)))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	if dest != source && false == isInside(dest, source) {
		_ = server.store.Rename(source, dest)
	}

	// 3. Return the refreshed listing.
	server.renderListPartial(writer, request, current)
}

// isInside reports whether child is the same as or nested under parent.
func isInside(child string, parent string) bool {

	return child == parent || strings.HasPrefix(child, parent+"/")
}

// handleBatchDelete removes every selected path, then returns the refreshed
// listing. Selected paths arrive as repeated "paths" form fields.
func (server *Server) handleBatchDelete(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var err error = request.ParseForm()
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusBadRequest)
		return
	}

	// 1. Resolve the directory to refresh.
	var parent string

	parent, err = server.store.Clean(request.FormValue("dir"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	// 2. Delete each selected path (skip any that fail to clean).
	var paths []string = request.Form["paths"]

	for _, raw := range paths {

		var target string
		var cleanErr error

		target, cleanErr = server.store.Clean(raw)
		if nil != cleanErr {
			continue
		}

		_ = server.store.Remove(target)
	}

	// 3. Return the refreshed listing.
	server.renderListPartial(writer, request, parent)
}

// handleRenameForm returns the inline rename edit row for a single entry.
func (server *Server) handleRenameForm(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var target string
	var err error

	target, err = server.store.Clean(request.URL.Query().Get("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var dir string

	dir, err = server.store.Clean(request.URL.Query().Get("dir"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	server.render(writer, request, http.StatusOK, view.RenameRow(path.Base(target), target, dir))
}

// handleRename renames or moves an entry, then returns the refreshed listing.
// A bare name renames in place; a value containing "/" (or a leading "/") moves
// the entry, mirroring the REST PATCH semantics.
func (server *Server) handleRename(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	// 1. Validate the source path and the directory to refresh.
	var source string
	var err error

	source, err = server.store.Clean(request.FormValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var parent string

	parent, err = server.store.Clean(request.FormValue("dir"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	// 2. Resolve the destination: absolute path, or a name relative to the
	//    source's own directory.
	var rawName string = strings.TrimSpace(request.FormValue("name"))

	if "" == rawName {
		http.Error(writer, translator.T("error.generic"), http.StatusBadRequest)
		return
	}

	var destinationInput string

	if strings.HasPrefix(rawName, "/") {
		destinationInput = rawName
	} else {
		destinationInput = path.Join(path.Dir(source), rawName)
	}

	var target string

	target, err = server.store.Clean(destinationInput)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	// 3. Rename and refresh the listing.
	err = server.store.Rename(source, target)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	server.renderListPartial(writer, request, parent)
}

// handleRaw is the static access surface (like nginx): a file is served as raw
// bytes with Range support; a directory returns a plain autoindex listing.
func (server *Server) handleRaw(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var cleaned string
	var err error

	cleaned, err = server.store.Clean("/" + request.PathValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	if info.IsDir() {
		server.writeRawIndex(writer, request, cleaned)
		return
	}

	server.serveFile(writer, request, cleaned, false)
}

// writeRawIndex renders an nginx-style directory listing for /raw: a <pre> block
// of links with each entry's modification time and size. The page has no app
// chrome or scripts 鈥?just the listing.
func (server *Server) writeRawIndex(writer http.ResponseWriter, request *http.Request, cleaned string) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var entries []storage.Entry
	var err error

	entries, err = server.store.List(cleaned)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	var builder strings.Builder

	builder.WriteString("<pre>\n")

	// Parent link, except at the root.
	if "/" != cleaned {
		builder.WriteString("<a href=\"")
		builder.WriteString(html.EscapeString(view.RawURL(path.Dir(cleaned))))
		builder.WriteString("\">../</a>\n")
	}

	for _, entry := range entries {

		var name string = entry.Name

		if entry.IsDir {
			name = name + "/"
		}

		var href string = view.RawURL(path.Join(cleaned, entry.Name))

		// 1. The link.
		builder.WriteString("<a href=\"")
		builder.WriteString(html.EscapeString(href))
		builder.WriteString("\">")
		builder.WriteString(html.EscapeString(name))
		builder.WriteString("</a>")

		// 2. Pad the name column, then append the date and size (nginx layout).
		var pad int = rawIndexNameWidth - utf8.RuneCountInString(name)

		if pad < 1 {
			pad = 1
		}

		builder.WriteString(strings.Repeat(" ", pad))
		builder.WriteString(entry.ModTime.Format("02-Jan-2006 15:04"))

		var sizeText string = "-"

		if false == entry.IsDir {
			sizeText = storage.FormatSize(entry.Size)
		}

		builder.WriteString(fmt.Sprintf("%20s\n", sizeText))
	}

	builder.WriteString("</pre>")

	var data view.RawIndexData

	data.Dir = cleaned
	data.Body = builder.String()

	server.render(writer, request, http.StatusOK, view.RawIndex(data))
}

// handleDownload serves a file as a forced download.
func (server *Server) handleDownload(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var cleaned string
	var err error

	cleaned, err = server.store.Clean("/" + request.PathValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	// A directory is streamed as a ZIP archive so the browser downloads one
	// file that recreates the tree when unpacked.
	if info.IsDir() {
		var archiveName string = path.Base(cleaned)
		if "/" == cleaned {
			archiveName = "tdrive"
		}

		writer.Header().Set("Content-Type", "application/zip")
		writer.Header().Set("Content-Disposition", contentDisposition(archiveName+".zip"))

		var zipErr = streamDirZip(server.store, cleaned, writer)
		if nil != zipErr {
			slog.Warn("directory zip failed", slog.String("error", zipErr.Error()))
			return
		}
		return
	}

	server.serveFile(writer, request, "/"+request.PathValue("path"), true)
}

// serveFile streams a file using http.ServeContent, which handles Range requests
// for resumable, streaming downloads of large files.
func (server *Server) serveFile(writer http.ResponseWriter, request *http.Request, rawPath string, asAttachment bool) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	// 1. Validate the path and confirm it is a regular file.
	var cleaned string
	var err error

	cleaned, err = server.store.Clean(rawPath)
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr || info.IsDir() {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	// 2. Open the file (afero.File satisfies io.ReadSeeker for ServeContent).
	var file afero.File

	file, err = server.store.Open(cleaned)
	if nil != err {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	defer file.Close()

	// 3. Set an attachment header for downloads.
	if asAttachment {
		writer.Header().Set("Content-Disposition", contentDisposition(info.Name()))
	}

	// 4. Stream the content with full Range support.
	http.ServeContent(writer, request, info.Name(), info.ModTime(), file)
}

// handleView renders the online preview page for a file.
func (server *Server) handleView(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	// 1. Validate the path and confirm it is a regular file.
	var cleaned string
	var err error

	cleaned, err = server.store.Clean("/" + request.PathValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr || info.IsDir() {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	// 2. Detect the preview kind and load text content when applicable.
	var data view.ViewerData

	data.Name = info.Name()
	data.Path = cleaned
	data.DirPath = path.Dir(cleaned)
	data.Kind, data.Content = server.previewContent(cleaned, info.Name())
	data.Editable = isEditableKind(data.Kind) && false == server.config.ReadOnly

	if "text" == data.Kind {
		data.HighlightLang = highlightLanguage(info.Name())
	}

	data.Page = server.pageContext(request, info.Name())

	server.render(writer, request, http.StatusOK, view.ViewerPage(data))
}

// isEditableKind reports whether a preview kind is text-based and thus editable.
func isEditableKind(kind string) bool {

	return "markdown" == kind || "html" == kind || "text" == kind
}

// previewContent determines the preview kind ("markdown", "html", "image",
// "text", or "none"). Markdown and HTML are rendered in the browser (the client
// fetches the raw bytes), so no text content is returned for them.
func (server *Server) previewContent(cleaned string, name string) (string, string) {

	if isImageExtension(name) {
		return "image", ""
	}

	if isMarkdownExtension(name) {
		return "markdown", ""
	}

	if isHtmlExtension(name) {
		return "html", ""
	}

	var file afero.File
	var err error

	file, err = server.store.Open(cleaned)
	if nil != err {
		return "none", ""
	}

	defer file.Close()

	// Read a small sample to decide whether the content is text.
	var sample []byte = make([]byte, 512)

	var read int

	read, _ = io.ReadFull(file, sample)
	sample = sample[:read]

	var treatAsText bool = isTextExtension(name) || looksLikeText(sample)

	if false == treatAsText {
		return "none", ""
	}

	// Re-read from the start, bounded by the preview limit.
	_, err = file.Seek(0, io.SeekStart)
	if nil != err {
		return "none", ""
	}

	var limited io.Reader = io.LimitReader(file, maxTextPreview)

	var content []byte

	content, err = io.ReadAll(limited)
	if nil != err {
		return "none", ""
	}

	return "text", string(content)
}

// handleEditForm renders the in-browser editor for a text-based file.
func (server *Server) handleEditForm(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var cleaned string
	var err error

	cleaned, err = server.store.Clean("/" + request.PathValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var info, statErr = server.store.Stat(cleaned)
	if nil != statErr || info.IsDir() {
		http.Error(writer, translator.T("error.notfound"), http.StatusNotFound)
		return
	}

	// Only text-based files are editable; otherwise fall back to the viewer.
	var kind string
	var content string
	var editable bool

	kind, content, editable = server.editableContent(cleaned, info.Name())
	if false == editable {
		http.Redirect(writer, request, view.ViewURL(cleaned), http.StatusFound)
		return
	}

	var data view.EditData

	data.Name = info.Name()
	data.Path = cleaned
	data.Kind = kind
	data.Content = content
	data.Page = server.pageContext(request, info.Name())

	server.render(writer, request, http.StatusOK, view.EditPage(data))
}

// editableContent classifies a file for editing and reads its text. The kind is
// "markdown", "html", or "text"; editable is false for binary content.
func (server *Server) editableContent(cleaned string, name string) (string, string, bool) {

	var file afero.File
	var err error

	file, err = server.store.Open(cleaned)
	if nil != err {
		return "", "", false
	}

	defer file.Close()

	var sample []byte = make([]byte, 512)

	var read int

	read, _ = io.ReadFull(file, sample)
	sample = sample[:read]

	var treatAsText bool = isTextExtension(name) || isMarkdownExtension(name) || isHtmlExtension(name) || looksLikeText(sample)

	if false == treatAsText {
		return "", "", false
	}

	_, err = file.Seek(0, io.SeekStart)
	if nil != err {
		return "", "", false
	}

	var content []byte

	content, err = io.ReadAll(io.LimitReader(file, maxTextPreview))
	if nil != err {
		return "", "", false
	}

	var kind string = "text"

	if isMarkdownExtension(name) {
		kind = "markdown"
	} else if isHtmlExtension(name) {
		kind = "html"
	}

	return kind, string(content), true
}

// handleEditSave writes the edited content back to the file and returns to the viewer.
func (server *Server) handleEditSave(writer http.ResponseWriter, request *http.Request) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var cleaned string
	var err error

	cleaned, err = server.store.Clean("/" + request.PathValue("path"))
	if nil != err {
		http.Error(writer, translator.T("error.forbidden"), http.StatusForbidden)
		return
	}

	var content string = request.FormValue("content")

	_, err = server.store.WriteFile(cleaned, strings.NewReader(content))
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	http.Redirect(writer, request, view.ViewURL(cleaned), http.StatusFound)
}

// handleLoginForm renders the password gate.
func (server *Server) handleLoginForm(writer http.ResponseWriter, request *http.Request) {

	// With no password configured there is nothing to log in to.
	if false == server.config.AuthEnabled() {
		http.Redirect(writer, request, "/", http.StatusFound)
		return
	}

	var data view.LoginData

	data.Next = sanitizeNext(request.URL.Query().Get("next"))
	data.ShowError = "1" == request.URL.Query().Get("error")
	data.Page = server.pageContext(request, meta.Name)

	server.render(writer, request, http.StatusOK, view.LoginPage(data))
}

// handleLoginSubmit verifies the password and starts a session.
func (server *Server) handleLoginSubmit(writer http.ResponseWriter, request *http.Request) {

	var next string = sanitizeNext(request.FormValue("next"))
	var password string = request.FormValue("password")

	// 1. Compare the password in constant time.
	var match bool = 1 == subtle.ConstantTimeCompare([]byte(password), []byte(server.config.Password))

	if false == match {
		http.Redirect(writer, request, "/login?error=1&next="+queryEscape(next), http.StatusFound)
		return
	}

	// 2. Issue a session token and set the cookie.
	var token string
	var err error

	token, err = server.sessions.issue()
	if nil != err {
		http.Error(writer, "login error", http.StatusInternalServerError)
		return
	}

	server.setSessionCookie(writer, token)

	http.Redirect(writer, request, next, http.StatusFound)
}

// handleLogout revokes the session and clears the cookie.
func (server *Server) handleLogout(writer http.ResponseWriter, request *http.Request) {

	var cookie, err = request.Cookie(meta.SessionCookie())

	if nil == err {
		server.sessions.revoke(cookie.Value)
	}

	server.clearSessionCookie(writer)

	http.Redirect(writer, request, "/login", http.StatusFound)
}

// handleLanguage remembers the chosen language and redirects back.
func (server *Server) handleLanguage(writer http.ResponseWriter, request *http.Request) {

	var code string = request.PathValue("code")

	if server.bundle.Supports(code) {

		var cookie http.Cookie

		cookie.Name = meta.LanguageCookie()
		cookie.Value = code
		cookie.Path = "/"
		cookie.MaxAge = int((365 * 24 * time.Hour).Seconds())
		cookie.HttpOnly = false
		cookie.SameSite = http.SameSiteLaxMode

		http.SetCookie(writer, &cookie)
	}

	var next string = sanitizeNext(request.URL.Query().Get("next"))

	http.Redirect(writer, request, next, http.StatusFound)
}

// handleUploadInit starts a chunked upload session.
func (server *Server) handleUploadInit(writer http.ResponseWriter, request *http.Request) {

	var body uploadInitRequest
	var err error

	err = json.NewDecoder(request.Body).Decode(&body)
	if nil != err {
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}

	var uploadId string

	uploadId, err = server.uploads.init(body)
	if nil != err {
		slog.Warn("upload init failed", slog.String("error", err.Error()))
		http.Error(writer, "init failed", http.StatusBadRequest)
		return
	}

	writeJson(writer, map[string]string{"uploadId": uploadId})
}

// handleUploadChunk stores one chunk of an in-progress upload.
func (server *Server) handleUploadChunk(writer http.ResponseWriter, request *http.Request) {

	var uploadId string = request.URL.Query().Get("uploadId")

	var index int
	var err error

	index, err = strconv.Atoi(request.URL.Query().Get("index"))
	if nil != err {
		http.Error(writer, "invalid index", http.StatusBadRequest)
		return
	}

	err = server.uploads.writeChunk(uploadId, index, request.Body)
	if nil != err {
		slog.Warn("upload chunk failed", slog.String("error", err.Error()))
		http.Error(writer, "chunk failed", http.StatusBadRequest)
		return
	}

	writer.WriteHeader(http.StatusOK)
}

// handleUploadComplete assembles the chunks into the final file.
func (server *Server) handleUploadComplete(writer http.ResponseWriter, request *http.Request) {

	var uploadId string = request.URL.Query().Get("uploadId")

	var target string
	var err error

	target, err = server.uploads.complete(uploadId)
	if nil != err {
		slog.Warn("upload complete failed", slog.String("error", err.Error()))
		http.Error(writer, "complete failed", http.StatusBadRequest)
		return
	}

	slog.Info("upload complete", slog.String("path", target))

	writeJson(writer, map[string]string{"path": target})
}

// renderListPartial renders the file-list fragment for a directory.
func (server *Server) renderListPartial(writer http.ResponseWriter, request *http.Request, dir string) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var data view.BrowseData
	var err error

	data, err = server.buildBrowseData(request, dir)
	if nil != err {
		http.Error(writer, translator.T("error.generic"), http.StatusInternalServerError)
		return
	}

	server.render(writer, request, http.StatusOK, view.FileList(data))
}

// buildBrowseData assembles the model for a directory listing.
func (server *Server) buildBrowseData(request *http.Request, cleaned string) (view.BrowseData, error) {

	var translator *i18n.Translator = i18n.FromContext(request.Context())

	var data view.BrowseData

	data.Dir = cleaned
	data.ReadOnly = server.config.ReadOnly
	data.Crumbs = buildCrumbs(translator, cleaned)

	if "/" != cleaned {
		data.ParentPath = path.Dir(cleaned)
	}

	var entries []storage.Entry
	var err error

	entries, err = server.store.List(cleaned)
	if nil != err {
		return data, err
	}

	data.Rows = make([]view.FileRow, 0, len(entries))

	var totalSize int64 = 0

	for _, entry := range entries {

		var row view.FileRow

		row.Name = entry.Name
		row.Path = path.Join(cleaned, entry.Name)
		row.IsDir = entry.IsDir
		row.ModText = entry.ModTime.Format(modTimeLayout)
		row.ModUnix = entry.ModTime.Unix()
		row.ConfirmText = translator.Tf("delete.confirm", entry.Name)

		if false == entry.IsDir {
			row.Size = entry.Size
			row.SizeText = storage.FormatSize(entry.Size)
			row.Viewable = isViewable(entry.Name)
			row.IsImage = isImageExtension(entry.Name)
			totalSize = totalSize + entry.Size
		}

		data.Rows = append(data.Rows, row)
	}

	// README: render the directory's README below the listing, GitHub-style. Pick
	// the best candidate (README.md > README.markdown > README.txt) among entries.
	var readmeRank int = 0

	for _, entry := range entries {
		if entry.IsDir {
			continue
		}

		var kind string
		rank := 0

		kind, rank = readmeKind(entry.Name)

		if rank == 0 {
			continue
		}

		if data.Readme.Path == "" || rank < readmeRank {
			readmeRank = rank
			data.Readme = view.DirReadme{
				Path: path.Join(cleaned, entry.Name),
				Kind: kind,
				Name: entry.Name,
			}
		}
	}

	// Status line: item count and the combined size of the files in this folder.
	data.StatusText = translator.Tf("browse.item_count", len(entries)) + " \u00b7 " + storage.FormatSize(totalSize)

	return data, nil
}

// buildCrumbs builds breadcrumb segments from the data root to the current path.
func buildCrumbs(translator *i18n.Translator, cleaned string) []view.Crumb {

	var crumbs []view.Crumb

	crumbs = append(crumbs, view.Crumb{Name: translator.T("nav.home"), Path: "/"})

	if "/" == cleaned {
		return crumbs
	}

	var parts []string = strings.Split(strings.Trim(cleaned, "/"), "/")

	var current string = ""

	for _, part := range parts {

		current = current + "/" + part

		crumbs = append(crumbs, view.Crumb{Name: part, Path: current})
	}

	return crumbs
}

// setSessionCookie writes the session cookie.
func (server *Server) setSessionCookie(writer http.ResponseWriter, token string) {

	var cookie http.Cookie

	cookie.Name = meta.SessionCookie()
	cookie.Value = token
	cookie.Path = "/"
	cookie.MaxAge = int(sessionTTL.Seconds())
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode

	http.SetCookie(writer, &cookie)
}

// clearSessionCookie removes the session cookie.
func (server *Server) clearSessionCookie(writer http.ResponseWriter) {

	var cookie http.Cookie

	cookie.Name = meta.SessionCookie()
	cookie.Value = ""
	cookie.Path = "/"
	cookie.MaxAge = -1
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteStrictMode

	http.SetCookie(writer, &cookie)
}

// sanitizeNext keeps only safe in-site redirect targets to avoid open redirects.
func sanitizeNext(next string) string {

	if "" == next {
		return "/"
	}

	// Reject absolute URLs and protocol-relative URLs.
	if false == strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}

	return next
}

// contentDisposition builds an attachment header that handles non-ASCII names.
func contentDisposition(name string) string {

	// RFC 5987 expects percent-encoding; QueryEscape uses '+' for spaces, so fix that.
	var escaped string = strings.ReplaceAll(url.QueryEscape(name), "+", "%20")

	return "attachment; filename*=UTF-8''" + escaped
}

// queryEscape escapes a string for safe use as a URL query value.
func queryEscape(value string) string {

	return url.QueryEscape(value)
}

// writeJson writes a value as a JSON response.
func writeJson(writer http.ResponseWriter, value any) {

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")

	_ = json.NewEncoder(writer).Encode(value)
}
