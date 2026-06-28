package web

import (
	"path"
	"strings"
	"unicode/utf8"
)

// maxTextPreview bounds how many bytes of a text file are read for online preview.
const maxTextPreview int64 = 2 * 1024 * 1024

// textExtensions lists file extensions previewed as plain text.
var textExtensions map[string]bool = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".log": true,
	".json": true, ".yaml": true, ".yml": true, ".xml": true, ".toml": true,
	".ini": true, ".conf": true, ".cfg": true, ".env": true, ".properties": true,
	".csv": true, ".tsv": true, ".sql": true,
	".go": true, ".js": true, ".mjs": true, ".ts": true, ".jsx": true, ".tsx": true,
	".css": true, ".scss": true, ".less": true, ".html": true, ".htm": true,
	".sh": true, ".bash": true, ".zsh": true, ".ps1": true, ".bat": true,
	".py": true, ".rb": true, ".php": true, ".pl": true, ".lua": true,
	".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cc": true,
	".java": true, ".kt": true, ".rs": true, ".swift": true, ".cs": true,
	".gradle": true, ".dockerfile": true, ".makefile": true, ".mk": true,
}

// imageExtensions lists file extensions previewed as images.
var imageExtensions map[string]bool = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".ico": true, ".svg": true,
}

// isTextExtension reports whether a filename's extension is a known text type.
func isTextExtension(name string) bool {

	var ext string = strings.ToLower(path.Ext(name))

	return textExtensions[ext]
}

// isImageExtension reports whether a filename's extension is a known image type.
func isImageExtension(name string) bool {

	var ext string = strings.ToLower(path.Ext(name))

	return imageExtensions[ext]
}

// highlightLanguages maps a file extension to a highlight.js language id. Files
// with no entry are shown as plain monospace text (no error-prone auto-detect).
var highlightLanguages map[string]string = map[string]string{
	".go": "go", ".js": "javascript", ".mjs": "javascript", ".jsx": "javascript",
	".ts": "typescript", ".tsx": "typescript", ".py": "python", ".rb": "ruby",
	".php": "php", ".java": "java", ".kt": "kotlin", ".rs": "rust",
	".c": "c", ".h": "c", ".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp",
	".cs": "csharp", ".swift": "swift", ".sh": "bash", ".bash": "bash",
	".zsh": "bash", ".ps1": "powershell", ".json": "json", ".yaml": "yaml",
	".yml": "yaml", ".toml": "toml", ".xml": "xml", ".css": "css",
	".scss": "scss", ".less": "less", ".sql": "sql", ".ini": "ini",
	".conf": "ini", ".cfg": "ini", ".lua": "lua", ".pl": "perl",
	".gradle": "gradle", ".mk": "makefile", ".dockerfile": "dockerfile",
}

// highlightLanguage returns the highlight.js language id for a filename, or "".
func highlightLanguage(name string) string {

	var ext string = strings.ToLower(path.Ext(name))

	return highlightLanguages[ext]
}

// isMarkdownExtension reports whether a filename is Markdown.
func isMarkdownExtension(name string) bool {

	var ext string = strings.ToLower(path.Ext(name))

	return ".md" == ext || ".markdown" == ext
}

// isHtmlExtension reports whether a filename is HTML.
func isHtmlExtension(name string) bool {

	var ext string = strings.ToLower(path.Ext(name))

	return ".html" == ext || ".htm" == ext
}

// isViewable reports whether a file can be previewed online (text or image).
func isViewable(name string) bool {

	return isTextExtension(name) || isImageExtension(name)
}

// looksLikeText reports whether a byte sample is printable UTF-8 text. It is used
// as a fallback for files whose extension is unknown.
func looksLikeText(sample []byte) bool {

	if 0 == len(sample) {
		return true
	}

	// A NUL byte is a strong signal of binary content.
	for _, b := range sample {
		if 0 == b {
			return false
		}
	}

	return utf8.Valid(sample)
}

// readmeKind reports the render kind for a README candidate filename, and a rank
// where lower is preferred. GitHub-like precedence: README.md beats
// README.markdown beats README.txt. A non-README file returns ("", 0). The name
// check is case-insensitive on the stem and the extension.
func readmeKind(name string) (string, int) {

	var lower string = strings.ToLower(name)

	if !strings.HasPrefix(lower, "readme.") {
		return "", 0
	}

	switch lower {
	case "readme.md":
		return "markdown", 1
	case "readme.markdown":
		return "markdown", 2
	case "readme.txt":
		return "text", 3
	}

	return "", 0
}
