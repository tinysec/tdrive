package web

import "testing"

// TestSanitizeName verifies client file names are reduced to a safe base name.
func TestSanitizeName(t *testing.T) {

	var valid map[string]string = map[string]string{
		"a.txt":       "a.txt",
		"/etc/passwd": "passwd",
		"../secret":   "secret",
		"a/b/c.txt":   "c.txt",
		"\\win\\f.md": "f.md",
	}

	for raw, expected := range valid {

		var got string
		var err error

		got, err = sanitizeName(raw)
		if nil != err {
			t.Errorf("sanitizeName(%q) unexpected error: %v", raw, err)
			continue
		}

		if got != expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", raw, got, expected)
		}
	}

	var invalid []string = []string{"", ".", "..", "/", "   "}

	for _, raw := range invalid {

		var _, err = sanitizeName(raw)
		if nil == err {
			t.Errorf("sanitizeName(%q) should error", raw)
		}
	}
}

// TestLooksLikeText distinguishes printable UTF-8 from binary content.
func TestLooksLikeText(t *testing.T) {

	if false == looksLikeText([]byte("hello 世界\n")) {
		t.Error("expected UTF-8 text to be detected as text")
	}

	if looksLikeText([]byte{0x00, 0x01, 0x02}) {
		t.Error("expected NUL bytes to be detected as binary")
	}

	if false == looksLikeText([]byte{}) {
		t.Error("expected empty content to be treated as text")
	}
}
