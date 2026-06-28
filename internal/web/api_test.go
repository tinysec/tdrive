package web_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tdrive/internal/config"
	"tdrive/internal/i18n"
	"tdrive/internal/storage"
	"tdrive/internal/web"
)

// newTestServer starts an in-process HTTP server backed by a temp data root and
// returns it together with that root directory.
func newTestServer(t *testing.T, password string, readOnly bool) (*httptest.Server, string) {

	var root string = t.TempDir()

	var store *storage.Store
	var err error

	store, _, err = storage.New(root)
	if nil != err {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	var bundle *i18n.Bundle

	bundle, err = i18n.NewBundle()
	if nil != err {
		t.Fatalf("load bundle: %v", err)
	}

	var cfg *config.Config = config.New()

	cfg.Root = root
	cfg.Password = password
	cfg.ReadOnly = readOnly

	err = cfg.Validate()
	if nil != err {
		t.Fatalf("validate config: %v", err)
	}

	var server *web.Server = web.NewServer(cfg, store, bundle)

	var testServer *httptest.Server = httptest.NewServer(server.Handler())

	t.Cleanup(testServer.Close)

	return testServer, root
}

// request performs an HTTP request and returns the response and its body bytes.
func request(t *testing.T, method string, url string, body string) (*http.Response, []byte) {

	var reader io.Reader

	if "" != body {
		reader = strings.NewReader(body)
	}

	var httpRequest *http.Request
	var err error

	httpRequest, err = http.NewRequest(method, url, reader)
	if nil != err {
		t.Fatalf("build request: %v", err)
	}

	var response *http.Response

	response, err = http.DefaultClient.Do(httpRequest)
	if nil != err {
		t.Fatalf("do request: %v", err)
	}

	var data []byte

	data, _ = io.ReadAll(response.Body)
	response.Body.Close()

	return response, data
}

// TestRestApiLifecycle exercises the full file lifecycle through /api/v1.
func TestRestApiLifecycle(t *testing.T) {

	var server *httptest.Server
	var root string

	server, root = newTestServer(t, "", false)

	var base string = server.URL + "/api/v1"

	// 1. PUT a file (create) → 201 with JSON metadata.
	var response *http.Response
	var body []byte

	response, body = request(t, http.MethodPut, base+"/files/docs/note.txt", "hello world")
	if http.StatusCreated != response.StatusCode {
		t.Fatalf("PUT status = %d, want 201 (%s)", response.StatusCode, body)
	}

	var meta map[string]any

	_ = json.Unmarshal(body, &meta)
	if "file" != meta["type"] {
		t.Errorf("PUT metadata type = %v, want file", meta["type"])
	}

	// 2. File actually written to disk with the right content.
	var onDisk []byte
	var err error

	onDisk, err = os.ReadFile(filepath.Join(root, "docs", "note.txt"))
	if nil != err || "hello world" != string(onDisk) {
		t.Errorf("file on disk = %q (err %v), want \"hello world\"", string(onDisk), err)
	}

	// 3. GET bytes from /content.
	response, body = request(t, http.MethodGet, base+"/content/docs/note.txt", "")
	if http.StatusOK != response.StatusCode || "hello world" != string(body) {
		t.Errorf("GET content = %d %q, want 200 \"hello world\"", response.StatusCode, body)
	}

	// 4. GET metadata from /files is JSON, never bytes.
	response, body = request(t, http.MethodGet, base+"/files/docs/note.txt", "")
	if http.StatusOK != response.StatusCode {
		t.Errorf("GET files meta status = %d, want 200", response.StatusCode)
	}
	if false == strings.Contains(string(body), "\"contentUrl\":\"/api/v1/content/docs/note.txt\"") {
		t.Errorf("GET files meta missing contentUrl: %s", body)
	}

	// 5. Directory listing.
	response, body = request(t, http.MethodGet, base+"/files/docs", "")
	if http.StatusOK != response.StatusCode || false == strings.Contains(string(body), "\"name\":\"note.txt\"") {
		t.Errorf("GET listing = %d %s", response.StatusCode, body)
	}

	// 6. MKCOL creates a sub-directory.
	response, _ = request(t, "MKCOL", base+"/files/docs/sub", "")
	if http.StatusCreated != response.StatusCode {
		t.Errorf("MKCOL status = %d, want 201", response.StatusCode)
	}
	var dirInfo os.FileInfo
	dirInfo, err = os.Stat(filepath.Join(root, "docs", "sub"))
	if nil != err || false == dirInfo.IsDir() {
		t.Errorf("sub directory not created on disk (err %v)", err)
	}

	// 7. PATCH moves the file.
	response, _ = request(t, http.MethodPatch, base+"/files/docs/note.txt", "{\"path\":\"/docs/sub/moved.txt\"}")
	if http.StatusOK != response.StatusCode {
		t.Errorf("PATCH move status = %d, want 200", response.StatusCode)
	}
	response, body = request(t, http.MethodGet, base+"/content/docs/sub/moved.txt", "")
	if http.StatusOK != response.StatusCode || "hello world" != string(body) {
		t.Errorf("moved file content = %d %q", response.StatusCode, body)
	}

	// 8. DELETE removes it (204), then it is gone (404).
	response, _ = request(t, http.MethodDelete, base+"/files/docs/sub/moved.txt", "")
	if http.StatusNoContent != response.StatusCode {
		t.Errorf("DELETE status = %d, want 204", response.StatusCode)
	}
	response, _ = request(t, http.MethodGet, base+"/files/docs/sub/moved.txt", "")
	if http.StatusNotFound != response.StatusCode {
		t.Errorf("GET after delete = %d, want 404", response.StatusCode)
	}
}

// TestRestApiRange verifies partial content download from /content.
func TestRestApiRange(t *testing.T) {

	var server *httptest.Server

	server, _ = newTestServer(t, "", false)

	var base string = server.URL + "/api/v1"

	request(t, http.MethodPut, base+"/files/r.txt", "0123456789")

	var httpRequest *http.Request
	var err error

	httpRequest, err = http.NewRequest(http.MethodGet, base+"/content/r.txt", nil)
	if nil != err {
		t.Fatalf("build request: %v", err)
	}

	httpRequest.Header.Set("Range", "bytes=0-2")

	var response *http.Response

	response, err = http.DefaultClient.Do(httpRequest)
	if nil != err {
		t.Fatalf("do request: %v", err)
	}

	var data []byte

	data, _ = io.ReadAll(response.Body)
	response.Body.Close()

	if http.StatusPartialContent != response.StatusCode {
		t.Errorf("Range status = %d, want 206", response.StatusCode)
	}
	if "012" != string(data) {
		t.Errorf("Range body = %q, want \"012\"", data)
	}
}

// TestRestApiAuth verifies Basic Auth gating when a password is set.
func TestRestApiAuth(t *testing.T) {

	var server *httptest.Server

	server, _ = newTestServer(t, "s3cret", false)

	var url string = server.URL + "/api/v1/files"

	// 1. No credentials → 401.
	var response *http.Response

	response, _ = request(t, http.MethodGet, url, "")
	if http.StatusUnauthorized != response.StatusCode {
		t.Errorf("no-auth status = %d, want 401", response.StatusCode)
	}

	// 2. Correct password via Basic Auth → 200.
	var httpRequest *http.Request
	var err error

	httpRequest, err = http.NewRequest(http.MethodGet, url, nil)
	if nil != err {
		t.Fatalf("build request: %v", err)
	}

	httpRequest.SetBasicAuth("anyuser", "s3cret")

	response, err = http.DefaultClient.Do(httpRequest)
	if nil != err {
		t.Fatalf("do request: %v", err)
	}

	response.Body.Close()

	if http.StatusOK != response.StatusCode {
		t.Errorf("basic-auth status = %d, want 200", response.StatusCode)
	}

	// 3. Wrong password → 401.
	httpRequest, err = http.NewRequest(http.MethodGet, url, nil)
	if nil != err {
		t.Fatalf("build request: %v", err)
	}

	httpRequest.SetBasicAuth("anyuser", "wrong")

	response, err = http.DefaultClient.Do(httpRequest)
	if nil != err {
		t.Fatalf("do request: %v", err)
	}

	response.Body.Close()

	if http.StatusUnauthorized != response.StatusCode {
		t.Errorf("wrong-password status = %d, want 401", response.StatusCode)
	}
}

// TestRestApiReadOnly verifies that read-only mode allows reads but rejects every
// write method with 405 across the REST API.
func TestRestApiReadOnly(t *testing.T) {

	var server *httptest.Server
	var root string

	server, root = newTestServer(t, "", true)

	// Seed a file directly on disk (bypassing the read-only HTTP surface).
	var err error = os.WriteFile(filepath.Join(root, "seed.txt"), []byte("data"), 0o600)
	if nil != err {
		t.Fatalf("seed file: %v", err)
	}

	var base string = server.URL + "/api/v1"

	// Reads succeed.
	var response *http.Response

	response, _ = request(t, http.MethodGet, base+"/files/seed.txt", "")
	if http.StatusOK != response.StatusCode {
		t.Errorf("read-only GET metadata = %d, want 200", response.StatusCode)
	}

	response, _ = request(t, http.MethodGet, base+"/content/seed.txt", "")
	if http.StatusOK != response.StatusCode {
		t.Errorf("read-only GET content = %d, want 200", response.StatusCode)
	}

	// Every write is rejected with 405.
	var writes []struct {
		method string
		path   string
	} = []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/files/new.txt"},
		{"MKCOL", "/files/dir"},
		{http.MethodPatch, "/files/seed.txt"},
		{http.MethodDelete, "/files/seed.txt"},
	}

	for _, write := range writes {

		response, _ = request(t, write.method, base+write.path, "")
		if http.StatusMethodNotAllowed != response.StatusCode {
			t.Errorf("read-only %s %s = %d, want 405", write.method, write.path, response.StatusCode)
		}
	}
}

// TestEditFlow verifies the in-browser editor loads and saves file content.
func TestEditFlow(t *testing.T) {

	var server *httptest.Server
	var root string

	server, root = newTestServer(t, "", false)

	var err error = os.WriteFile(filepath.Join(root, "note.md"), []byte("# old"), 0o600)
	if nil != err {
		t.Fatalf("seed file: %v", err)
	}

	// 1. The editor page loads.
	var response *http.Response

	response, _ = request(t, http.MethodGet, server.URL+"/edit/note.md", "")
	if http.StatusOK != response.StatusCode {
		t.Errorf("GET /edit status = %d, want 200", response.StatusCode)
	}

	// 2. Saving writes the new content (PostForm sets the form content type).
	var form url.Values = url.Values{}

	form.Set("content", "# new content")

	response, err = http.PostForm(server.URL+"/edit/note.md", form)
	if nil != err {
		t.Fatalf("post form: %v", err)
	}

	response.Body.Close()

	var saved []byte

	saved, err = os.ReadFile(filepath.Join(root, "note.md"))
	if nil != err || "# new content" != string(saved) {
		t.Errorf("saved content = %q (err %v), want \"# new content\"", string(saved), err)
	}
}

// TestBatchDelete verifies multiple selected paths are deleted in one request.
func TestBatchDelete(t *testing.T) {

	var server *httptest.Server
	var root string

	server, root = newTestServer(t, "", false)

	var names []string = []string{"a.txt", "b.txt", "c.txt"}

	for _, name := range names {

		var err error = os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600)
		if nil != err {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	var form url.Values = url.Values{}

	form.Set("dir", "/")
	form.Add("paths", "/a.txt")
	form.Add("paths", "/b.txt")

	var response, err = http.PostForm(server.URL+"/api/batch-delete", form)
	if nil != err {
		t.Fatalf("post form: %v", err)
	}

	response.Body.Close()

	if http.StatusOK != response.StatusCode {
		t.Errorf("batch-delete status = %d, want 200", response.StatusCode)
	}

	assertMissing(t, filepath.Join(root, "a.txt"))
	assertMissing(t, filepath.Join(root, "b.txt"))

	var _, statErr = os.Stat(filepath.Join(root, "c.txt"))
	if nil != statErr {
		t.Errorf("c.txt should remain, got %v", statErr)
	}
}

// assertMissing fails if the path still exists.
func assertMissing(t *testing.T, path string) {

	var _, err = os.Stat(path)
	if false == os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted, stat err = %v", path, err)
	}
}

// TestMove verifies drag-and-drop move into a directory, and that a directory
// cannot be moved into itself.
func TestMove(t *testing.T) {

	var server *httptest.Server
	var root string

	server, root = newTestServer(t, "", false)

	var err error = os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o600)
	if nil != err {
		t.Fatalf("seed file: %v", err)
	}

	err = os.Mkdir(filepath.Join(root, "sub"), 0o755)
	if nil != err {
		t.Fatalf("seed dir: %v", err)
	}

	// Move /a.txt into /sub.
	var form url.Values = url.Values{}

	form.Set("path", "/a.txt")
	form.Set("dir", "/")
	form.Set("targetDir", "/sub")

	var response, postErr = http.PostForm(server.URL+"/api/move", form)
	if nil != postErr {
		t.Fatalf("post move: %v", postErr)
	}

	response.Body.Close()

	assertMissing(t, filepath.Join(root, "a.txt"))

	var _, statErr = os.Stat(filepath.Join(root, "sub", "a.txt"))
	if nil != statErr {
		t.Errorf("a.txt not moved into sub: %v", statErr)
	}

	// Moving /sub into itself must be a no-op (the directory still exists).
	var selfForm url.Values = url.Values{}

	selfForm.Set("path", "/sub")
	selfForm.Set("dir", "/")
	selfForm.Set("targetDir", "/sub")

	response, postErr = http.PostForm(server.URL+"/api/move", selfForm)
	if nil != postErr {
		t.Fatalf("post self-move: %v", postErr)
	}

	response.Body.Close()

	var info, infoErr = os.Stat(filepath.Join(root, "sub"))
	if nil != infoErr || false == info.IsDir() {
		t.Errorf("sub should still exist after self-move (err %v)", infoErr)
	}
}

// TestRouting checks the three access surfaces: / redirects to the UI, /raw
// serves a static directory autoindex, and /drive serves the web UI.
func TestRouting(t *testing.T) {

	var server *httptest.Server
	var root string

	server, root = newTestServer(t, "", false)

	var err error = os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi"), 0o600)
	if nil != err {
		t.Fatalf("seed file: %v", err)
	}

	// 1. / redirects to /drive/ (do not follow).
	var client http.Client

	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	var response *http.Response

	response, err = client.Get(server.URL + "/")
	if nil != err {
		t.Fatalf("get /: %v", err)
	}

	response.Body.Close()

	if http.StatusFound != response.StatusCode || "/drive/" != response.Header.Get("Location") {
		t.Errorf("/ = %d Location %q, want 302 /drive/", response.StatusCode, response.Header.Get("Location"))
	}

	// 2. /raw/ serves a static autoindex listing the file.
	var body []byte

	response, body = request(t, http.MethodGet, server.URL+"/raw/", "")
	if http.StatusOK != response.StatusCode {
		t.Errorf("/raw/ status = %d, want 200", response.StatusCode)
	}
	if false == strings.Contains(string(body), "Index of") || false == strings.Contains(string(body), "/raw/hello.txt") {
		t.Errorf("/raw/ autoindex missing entries: %s", body)
	}

	// 3. /drive/ serves the web UI.
	response, body = request(t, http.MethodGet, server.URL+"/drive/", "")
	if http.StatusOK != response.StatusCode {
		t.Errorf("/drive/ status = %d, want 200", response.StatusCode)
	}

	var driveBody string = string(body)
	if false == strings.Contains(driveBody, "\u00b7 2 B") {
		t.Errorf("/drive/ status text missing middle-dot separator: %s", driveBody)
	}
	if strings.Contains(driveBody, "\u8def") {
		t.Errorf("/drive/ status text contains wrong separator: %s", driveBody)
	}
}
