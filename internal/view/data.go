package view

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

// RowDragAttrs returns the drag-and-drop attributes for a file/dir row: every
// row is draggable, and directories are also drop targets. Empty in read-only mode.
func RowDragAttrs(row FileRow, readOnly bool) templ.Attributes {

	if readOnly {
		return templ.Attributes{}
	}

	var attrs templ.Attributes = templ.Attributes{
		"draggable":      "true",
		"data-drag-path": row.Path,
	}

	if row.IsDir {
		attrs["data-drop-dir"] = row.Path
	}

	return attrs
}

// ParentDropAttrs makes the "up one level" row a drop target for moving entries
// into the parent directory. Empty in read-only mode.
func ParentDropAttrs(parentPath string, readOnly bool) templ.Attributes {

	if readOnly {
		return templ.Attributes{}
	}

	return templ.Attributes{"data-drop-dir": parentPath}
}

// PageContext carries the per-request data the shared layout and navigation need.
type PageContext struct {
	// Title is the browser tab / header title (already localized).
	Title string

	// Lang is the active language code ("zh" or "en").
	Lang string

	// AuthEnabled indicates whether a password gate is configured (controls the logout link).
	AuthEnabled bool

	// CurrentURI is the request URI, used as the "next" target when switching language.
	CurrentURI string
}

// Crumb is one segment of the breadcrumb navigation.
type Crumb struct {
	Name string
	Path string
}

// FileRow is one row in a directory listing.
type FileRow struct {
	Name     string
	Path     string // canonical slash-rooted path, e.g. "/docs/readme.txt"
	IsDir    bool
	SizeText string
	ModText  string

	// Size and ModUnix are the raw values behind SizeText/ModText, exposed as
	// row data attributes so the client can sort and total without re-parsing
	// the formatted text. Size is 0 for directories.
	Size    int64
	ModUnix int64

	// Viewable is true when the file can be previewed online (text or image).
	Viewable bool

	// IsImage is true for image files, which show a thumbnail in grid view.
	IsImage bool

	// ConfirmText is the localized confirmation prompt shown before deletion.
	ConfirmText string
}

// RowDataAttrs emits the sort/filter data attributes for a row: the raw name,
// size, modification time, and whether it is a directory (folders sort first).
func RowDataAttrs(row FileRow) templ.Attributes {

	var isDir string = "0"

	if row.IsDir {
		isDir = "1"
	}

	return templ.Attributes{
		"data-name":  row.Name,
		"data-path":  row.Path,
		"data-size":  strconv.FormatInt(row.Size, 10),
		"data-mtime": strconv.FormatInt(row.ModUnix, 10),
		"data-dir":   isDir,
	}
}

// BrowseData is the model for the directory browsing page and its file-list partial.
type BrowseData struct {
	Page PageContext

	Dir        string // current canonical directory, "/" at the root
	ParentPath string // canonical parent directory; empty when already at the root
	Crumbs     []Crumb
	Rows       []FileRow

	// StatusText summarizes the listing for the status bar, e.g. "12 items \u00b7 1.2 GB"
	StatusText string

	// ReadOnly hides the write controls (upload, new folder, rename, delete).
	ReadOnly bool
	// Readme describes the directory's README file rendered below the listing,
	// like GitHub does. Empty when the directory has no README.
	Readme DirReadme
}

// ViewerData is the model for the file preview page.
type ViewerData struct {
	Page PageContext

	Name    string
	Path    string
	DirPath string // canonical directory containing the file (back link)

	// Kind is "markdown", "html", "text", "image", or "none".
	Kind string

	// Content holds the decoded text for Kind == "text" (markdown and HTML are
	// fetched and rendered in the browser, so they do not need it here).
	Content string

	// HighlightLang is the highlight.js language id for a code file, or "" for
	// plain text (no syntax highlighting).
	HighlightLang string

	// Editable is true when the file is text-based and the server is not read-only.
	Editable bool
}

// DirReadme describes a directory's README so the browse page can render it
// below the file list, mirroring GitHub. The page fetches the raw bytes and
// renders them client-side, so only the canonical path and render kind are needed.
type DirReadme struct {
	// Path is the canonical slash-rooted path of the README file (e.g. /docs/README.md).
	Path string

	// Kind is the render kind: "markdown" or "text".
	Kind string

	// Name is the README's own filename (e.g. README.md) for the section heading.
	Name string
}

// EditData is the model for the in-browser editor.
type EditData struct {
	Page PageContext

	Name string
	Path string

	// Kind is "markdown", "html", or "text"; it selects the live-preview style.
	Kind string

	// Content is the current file text shown in the editor.
	Content string
}

// LoginData is the model for the login page.
type LoginData struct {
	Page PageContext

	// ShowError is true when the previous attempt used a wrong password.
	ShowError bool

	// Next is the path to redirect to after a successful login.
	Next string
}

// RawIndexData is the model for the static, nginx-style directory index served
// under /raw. Body is pre-rendered HTML (the <pre> listing) built by the handler.
type RawIndexData struct {
	Dir  string
	Body string
}

// BrowseURL builds the link to the web UI for a directory at the given path.
func BrowseURL(canonical string) string {

	return joinURL("/drive", canonical)
}

// RawURL builds the link that serves a file's raw bytes (inline, static access).
func RawURL(canonical string) string {

	return joinURL("/raw", canonical)
}

// DownloadURL builds the link that serves a file as a forced download.
func DownloadURL(canonical string) string {

	return joinURL("/download", canonical)
}

// ViewURL builds the link to the online preview page for a file.
func ViewURL(canonical string) string {

	return joinURL("/view", canonical)
}

// EditURL builds the link to the in-browser editor for a file.
func EditURL(canonical string) string {

	return joinURL("/edit", canonical)
}

// LangURL builds the link that switches the UI language and returns to next.
func LangURL(code string, next string) string {

	var target url.URL

	target.Path = "/lang/" + code

	var query url.Values = url.Values{}

	query.Set("next", next)

	target.RawQuery = query.Encode()

	return target.String()
}

// PathDirVals builds the JSON htmx hx-vals payload carrying a target path and
// the directory to refresh. Used by the delete and rename actions.
func PathDirVals(path string, dir string) string {

	return jsonObject(map[string]string{
		"path": path,
		"dir":  dir,
	})
}

// DirVals builds the JSON htmx hx-vals payload carrying only a directory, used
// to refresh a listing (e.g. cancelling an inline edit).
func DirVals(dir string) string {

	return jsonObject(map[string]string{
		"dir": dir,
	})
}

// joinURL prefixes a route base in front of a canonical path with proper escaping.
func joinURL(base string, canonical string) string {

	var target url.URL

	target.Path = base + canonical

	return target.String()
}

// jsonObject marshals a small string map to a compact JSON object string.
func jsonObject(values map[string]string) string {

	var raw []byte
	var err error

	raw, err = json.Marshal(values)
	if nil != err {
		return "{}"
	}

	return string(raw)
}

// IsZipName reports whether a filename is a ZIP archive (.zip, case-insensitive).
func IsZipName(name string) bool {

	var lower string = strings.ToLower(name)

	return strings.HasSuffix(lower, ".zip")
}
