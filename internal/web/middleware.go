package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"tdrive/internal/i18n"
	"tdrive/internal/meta"
)

// contextKey is a private type for request-scoped values.
type contextKey int

const (
	contextRequestId contextKey = iota
)

// statusRecorder captures the response status code for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(code int) {

	recorder.status = code
	recorder.ResponseWriter.WriteHeader(code)
}

// Flush forwards flush calls so streaming responses are not buffered by the wrapper.
func (recorder *statusRecorder) Flush() {

	var flusher, ok = recorder.ResponseWriter.(http.Flusher)

	if ok {
		flusher.Flush()
	}
}

// recoverMiddleware turns a panic into a 500 response and logs the stack trace.
func (server *Server) recoverMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		defer func() {

			var recovered any = recover()

			if nil != recovered {

				slog.Error("panic recovered",
					slog.Any("error", recovered),
					slog.String("stack", string(debug.Stack())),
				)

				http.Error(writer, "internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(writer, request)
	})
}

// requestIdMiddleware assigns each request a request id and echoes it back.
func (server *Server) requestIdMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		var requestId string = request.Header.Get("X-Request-Id")

		if "" == requestId {
			requestId = newRequestId()
		}

		writer.Header().Set("X-Request-Id", requestId)

		var ctx context.Context = context.WithValue(request.Context(), contextRequestId, requestId)

		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// loggerMiddleware logs one structured line per request.
func (server *Server) loggerMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		var start time.Time = time.Now()

		var recorder *statusRecorder = &statusRecorder{ResponseWriter: writer, status: http.StatusOK}

		next.ServeHTTP(recorder, request)

		var requestId string = requestIdFromContext(request.Context())

		slog.Info("request",
			slog.String("request_id", requestId),
			slog.String("method", request.Method),
			slog.String("path", request.URL.Path),
			slog.Int("status", recorder.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// languageMiddleware resolves the UI language and binds a translator to the context.
// Priority: ?lang query > language cookie > configured default.
func (server *Server) languageMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		var lang string = server.resolveLanguage(request)

		var translator *i18n.Translator = server.bundle.Translator(lang)

		var ctx context.Context = i18n.WithTranslator(request.Context(), translator)

		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// resolveLanguage determines the active language for a request.
func (server *Server) resolveLanguage(request *http.Request) string {

	// 1. Explicit query override.
	var query string = request.URL.Query().Get("lang")

	if server.bundle.Supports(query) {
		return query
	}

	// 2. Remembered cookie.
	var cookie, err = request.Cookie(meta.LanguageCookie())

	if nil == err && server.bundle.Supports(cookie.Value) {
		return cookie.Value
	}

	// 3. Configured default.
	return server.config.Language
}

// authMiddleware enforces the optional password gate. It is fail-open when no
// password is configured; otherwise unauthenticated requests are redirected
// (for page views) or rejected with 401 (for API/asset requests).
func (server *Server) authMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		// 1. No password configured: fully open access.
		if false == server.config.AuthEnabled() {
			next.ServeHTTP(writer, request)
			return
		}

		// 2. Always-public paths needed to reach or render the login screen.
		if isPublicPath(request.URL.Path) {
			next.ServeHTTP(writer, request)
			return
		}

		// 3. Accept either a session cookie (browser) or HTTP Basic Auth password
		//    (REST API, WebDAV, scripts).
		if server.hasValidSession(request) || server.basicAuthOk(request) {
			next.ServeHTTP(writer, request)
			return
		}

		// 4. Reject. Page navigations redirect to the login screen; API/WebDAV/
		//    non-GET requests get a 401 with a Basic-Auth challenge.
		if http.MethodGet == request.Method && isPagePath(request.URL.Path) {
			redirectToLogin(writer, request)
			return
		}

		writer.Header().Set("WWW-Authenticate", "Basic realm=\""+meta.Name+"\"")
		http.Error(writer, "unauthorized", http.StatusUnauthorized)
	})
}

// basicAuthOk validates HTTP Basic Auth. Any username is accepted; only the
// password must match (single-user model).
func (server *Server) basicAuthOk(request *http.Request) bool {

	var _, password, ok = request.BasicAuth()

	if false == ok {
		return false
	}

	return 1 == subtle.ConstantTimeCompare([]byte(password), []byte(server.config.Password))
}

// hasValidSession reports whether the request carries a valid session cookie.
func (server *Server) hasValidSession(request *http.Request) bool {

	var cookie, err = request.Cookie(meta.SessionCookie())

	if nil != err {
		return false
	}

	return server.sessions.valid(cookie.Value)
}

// isPublicPath lists paths reachable without authentication.
func isPublicPath(path string) bool {

	if "/login" == path || "/health" == path {
		return true
	}

	if strings.HasPrefix(path, "/static/") {
		return true
	}

	if strings.HasPrefix(path, "/lang/") {
		return true
	}

	return false
}

// isPagePath reports whether a path is a browser page (so an unauthenticated GET
// should redirect to login) rather than an API/WebDAV endpoint (which gets 401).
func isPagePath(path string) bool {

	if strings.HasPrefix(path, "/api/") {
		return false
	}

	if strings.HasPrefix(path, "/webdav") {
		return false
	}

	return true
}

// redirectToLogin sends the browser to the login page, preserving the target URL.
func redirectToLogin(writer http.ResponseWriter, request *http.Request) {

	var target string = "/login?next=" + url.QueryEscape(request.URL.RequestURI())

	http.Redirect(writer, request, target, http.StatusFound)
}

// readOnlyMiddleware enforces read-only mode: when enabled, only read methods
// (GET/HEAD/OPTIONS and WebDAV PROPFIND) are allowed; every write method is
// rejected with 405. This one choke point covers the web UI mutations, the REST
// API, and WebDAV writes (FTP is made read-only at the filesystem level).
func (server *Server) readOnlyMiddleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

		if server.config.ReadOnly && false == isReadMethod(request.Method) {
			http.Error(writer, "read-only", http.StatusMethodNotAllowed)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

// isReadMethod reports whether an HTTP method only reads (never mutates) state.
func isReadMethod(method string) bool {

	switch method {

	case http.MethodGet, http.MethodHead, http.MethodOptions, "PROPFIND":
		return true
	}

	return false
}

// requestIdFromContext returns the request id bound to ctx, or empty string.
func requestIdFromContext(ctx context.Context) string {

	var value any = ctx.Value(contextRequestId)

	var requestId string
	var ok bool

	requestId, ok = value.(string)
	if ok {
		return requestId
	}

	return ""
}

// newRequestId generates a random hex request id.
func newRequestId() string {

	var raw []byte = make([]byte, 16)

	var _, err = rand.Read(raw)
	if nil != err {
		return "unknown"
	}

	return hex.EncodeToString(raw)
}
