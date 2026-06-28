package view

import (
	"path"
	"strings"
)

// iconBodies holds the inner markup of each named icon. Every icon is a 24x24
// line glyph (no fill, stroke = currentColor) so it inherits the surrounding
// text colour and sits flush with text. Keep one entry per icon, alphabetical
// within groups, so the set is easy to scan and extend.
var iconBodies map[string]string = map[string]string{

	// Navigation / chrome.
	"home":          `<path d="M3 11 12 3l9 8"/><path d="M5 10v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V10"/>`,
	"chevron-right": `<path d="m9 6 6 6-6 6"/>`,
	"chevron-down":  `<path d="m6 9 6 6 6-6"/>`,
	"corner-up":     `<path d="M20 17v-2a4 4 0 0 0-4-4H5"/><path d="m9 7-4 4 4 4"/>`,
	"search":        `<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>`,
	"view-list":     `<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3.5 6h.01"/><path d="M3.5 12h.01"/><path d="M3.5 18h.01"/>`,
	"view-grid":     `<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>`,
	"log-out":       `<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>`,

	// Toolbar actions.
	"plus":        `<path d="M12 5v14"/><path d="M5 12h14"/>`,
	"upload":      `<path d="M12 15V3"/><path d="m7 8 5-5 5 5"/><path d="M4 15v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4"/>`,
	"folder-plus": `<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M12 11v6"/><path d="M9 14h6"/>`,
	"file-plus":   `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><path d="M12 12v6"/><path d="M9 15h6"/>`,

	// Per-row actions.
	"eye":      `<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>`,
	"download": `<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>`,
	"pencil":   `<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>`,
	"trash":    `<path d="M3 6h18"/><path d="M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2"/><path d="m19 6-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>`,
	"check":    `<path d="M20 6 9 17l-5-5"/>`,
	"link":     `<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1"/>`,
	"x":        `<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`,
	"book": `<path d="M12 7v14"/><path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z"/>`,
	"code": `<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>`,
	"quote": `<path d="M17 6H3"/><path d="M21 12H8"/><path d="M21 18H8"/><path d="M3 12v6"/>`,
	"list-ul": `<line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/>`,
	"list-ol": `<line x1="10" y1="6" x2="21" y2="6"/><line x1="10" y1="12" x2="21" y2="12"/><line x1="10" y1="18" x2="21" y2="18"/><path d="M4 6h1v4"/><path d="M4 10h2"/><path d="M6 18H4c0-1 2-1.5 2-2.5S5 14 4 14.5"/>`,
	"table": `<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M3 15h18"/><path d="M12 3v18"/>`,
	"minus": `<path d="M5 12h14"/>`,
	"external-link": `<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`,
	"archive":     `<path d="M21 8v13H3V8"/><path d="M1 3h22v5H1z"/><path d="M10 12h4"/>`,

	// File / folder glyphs used in the listing.
	"folder":       `<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`,
	"folder-open":  `<path d="M3 8a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2"/><path d="m3 10 1.6 9.2A2 2 0 0 0 6.6 21h10.8a2 2 0 0 0 2-1.8L21 10z"/>`,
	"file":         `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/>`,
	"file-text":    `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><path d="M9 13h6"/><path d="M9 17h4"/>`,
	"file-code":    `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><path d="m10 12-2 2 2 2"/><path d="m14 12 2 2-2 2"/>`,
	"file-image":   `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><circle cx="10" cy="13" r="1.4"/><path d="m8 19 3-3 5 4"/>`,
	"file-archive": `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><path d="M11 5v1"/><path d="M11 8v1"/><path d="M11 11v1"/>`,
	"file-music":   `<path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><circle cx="10" cy="17" r="1.5"/><path d="M11.5 17v-4l3 .9"/>`,
}

// Icon returns the full inline SVG markup for a named icon, ready to be emitted
// verbatim. An unknown name falls back to the generic file glyph so a missing
// entry can never produce broken markup.
func Icon(name string) string {

	var body string
	var found bool

	body, found = iconBodies[name]
	if false == found {
		body = iconBodies["file"]
	}

	return `<svg class="ic" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` + body + `</svg>`
}

// imageExtensions, codeExtensions, archiveExtensions and audioExtensions and
// docExtensions classify a file name into the glyph that best represents it.
var imageExtensions map[string]bool = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	".svg": true, ".bmp": true, ".ico": true, ".avif": true, ".tiff": true,
}

var codeExtensions map[string]bool = map[string]bool{
	".go": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".c": true,
	".h": true, ".cpp": true, ".cc": true, ".hpp": true, ".py": true,
	".rs": true, ".java": true, ".rb": true, ".php": true, ".sh": true,
	".css": true, ".html": true, ".htm": true, ".xml": true, ".sql": true,
}

var archiveExtensions map[string]bool = map[string]bool{
	".zip": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true,
	".xz": true, ".7z": true, ".rar": true, ".zst": true,
}

var audioExtensions map[string]bool = map[string]bool{
	".mp3": true, ".flac": true, ".wav": true, ".ogg": true, ".m4a": true,
	".aac": true, ".wma": true,
}

var docExtensions map[string]bool = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".rst": true, ".log": true,
	".pdf": true, ".doc": true, ".docx": true, ".rtf": true, ".csv": true,
}

// FileIconName picks the glyph name for a listing row from its type and
// extension. Directories are always folders; files fall back to the generic
// file glyph when the extension is unrecognized.
func FileIconName(row FileRow) string {

	if row.IsDir {
		return "folder"
	}

	var ext string = strings.ToLower(path.Ext(row.Name))

	if imageExtensions[ext] {
		return "file-image"
	}

	if codeExtensions[ext] {
		return "file-code"
	}

	if archiveExtensions[ext] {
		return "file-archive"
	}

	if audioExtensions[ext] {
		return "file-music"
	}

	if docExtensions[ext] {
		return "file-text"
	}

	return "file"
}
