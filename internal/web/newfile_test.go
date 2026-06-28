package web_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestNewFileCreatesAndRedirects checks that POST /api/newfile creates an empty
// file and asks htmx (via HX-Redirect) to open it in the editor.
func TestNewFileCreatesAndRedirects(t *testing.T) {

	var server, root = newTestServer(t, "", false)

	var form url.Values = url.Values{}
	form.Set("dir", "/")
	form.Set("name", "notes.md")

	var response *http.Response
	var err error

	response, err = http.PostForm(server.URL+"/api/newfile", form)
	if nil != err {
		t.Fatalf("post newfile: %v", err)
	}

	defer response.Body.Close()

	if http.StatusNoContent != response.StatusCode {
		t.Fatalf("status = %d, want 204", response.StatusCode)
	}

	var redirect string = response.Header.Get("HX-Redirect")
	if "/edit/notes.md" != redirect {
		t.Errorf("HX-Redirect = %q, want /edit/notes.md", redirect)
	}

	// The empty file must exist on disk.
	var info os.FileInfo

	info, err = os.Stat(filepath.Join(root, "notes.md"))
	if nil != err {
		t.Fatalf("stat created file: %v", err)
	}

	if 0 != info.Size() {
		t.Errorf("new file size = %d, want 0", info.Size())
	}
}

// TestNewFileRefusesExisting verifies the endpoint will not overwrite an entry
// that already exists.
func TestNewFileRefusesExisting(t *testing.T) {

	var server, _ = newTestServer(t, "", false)

	var form url.Values = url.Values{}
	form.Set("dir", "/")
	form.Set("name", "dup.txt")

	var first *http.Response
	var err error

	first, err = http.PostForm(server.URL+"/api/newfile", form)
	if nil != err {
		t.Fatalf("first post: %v", err)
	}
	first.Body.Close()

	var second *http.Response

	second, err = http.PostForm(server.URL+"/api/newfile", form)
	if nil != err {
		t.Fatalf("second post: %v", err)
	}
	second.Body.Close()

	if http.StatusConflict != second.StatusCode {
		t.Errorf("second status = %d, want 409", second.StatusCode)
	}
}
