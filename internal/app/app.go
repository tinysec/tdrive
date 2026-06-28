package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/spf13/afero"

	"tdrive/internal/config"
	"tdrive/internal/ftp"
	"tdrive/internal/i18n"
	"tdrive/internal/storage"
	"tdrive/internal/web"
)

// shutdownTimeout bounds how long graceful shutdown waits for in-flight work.
const shutdownTimeout time.Duration = 10 * time.Second

// uploadMaxAge is how long an abandoned upload session is kept before cleanup.
const uploadMaxAge time.Duration = 24 * time.Hour

// janitorInterval is how often stale uploads are swept.
const janitorInterval time.Duration = 1 * time.Hour

// App is the composite of the enabled access surfaces (HTTP and/or FTP) over one
// shared data directory.
type App struct {
	config     *config.Config
	store      *storage.Store
	httpServer *http.Server
	httpAddr   string
	ftpServer  *ftp.Server

	janitorStop chan struct{}
}

// New builds the application from configuration: it opens the shared filesystem
// and constructs whichever servers are enabled.
func New(cfg *config.Config) (*App, error) {

	// 1. Open the shared, sandboxed data directory. The same filesystem handle is
	//    reused by the FTP driver so both surfaces see identical files.
	var store *storage.Store
	var fsHandle afero.Fs
	var err error

	store, fsHandle, err = storage.New(cfg.Root)
	if nil != err {
		return nil, err
	}

	// 2. Load the translation bundle.
	var bundle *i18n.Bundle

	bundle, err = i18n.NewBundle()
	if nil != err {
		_ = store.Close()
		return nil, err
	}

	var app App

	app.config = cfg
	app.store = store
	app.janitorStop = make(chan struct{})

	// 3. Configure the HTTP surface when enabled.
	if cfg.Http.Enabled {

		var webServer *web.Server = web.NewServer(cfg, store, bundle)

		app.httpServer = &http.Server{
			Handler: webServer.Handler(),
		}

		app.httpAddr = cfg.Http.Addr
	}

	// 4. Configure the FTP surface when enabled. In read-only mode the FTP driver
	//    gets a read-only view of the shared filesystem (HTTP/WebDAV writes are
	//    blocked separately by the read-only middleware).
	if cfg.Ftp.Enabled {

		var ftpFs afero.Fs = fsHandle

		if cfg.ReadOnly {
			ftpFs = afero.NewReadOnlyFs(fsHandle)
		}

		app.ftpServer = ftp.New(ftpFs, cfg.Ftp, cfg.Password)
	}

	return &app, nil
}

// Start binds listeners and begins serving in the background (non-blocking), so
// it fits the service-manager lifecycle.
func (app *App) Start() error {

	if nil != app.httpServer {

		// Bind first so a port conflict is reported immediately rather than in a goroutine.
		var listener net.Listener
		var err error

		listener, err = net.Listen("tcp", app.httpAddr)
		if nil != err {
			return fmt.Errorf("bind http %q: %w", app.httpAddr, err)
		}

		slog.Info("http server listening", slog.String("addr", app.httpAddr))

		go app.serveHttp(listener)
	}

	if nil != app.ftpServer {

		var err error = app.ftpServer.Start()
		if nil != err {
			return fmt.Errorf("start ftp: %w", err)
		}
	}

	// Sweep abandoned upload sessions in the background.
	go app.runJanitor()

	return nil
}

// runJanitor periodically removes stale, incomplete upload working directories.
func (app *App) runJanitor() {

	// Run once at startup, then on a fixed interval.
	_ = storage.CleanupUploadTemp(uploadMaxAge)

	var ticker *time.Ticker = time.NewTicker(janitorInterval)
	defer ticker.Stop()

	for {
		select {

		case <-ticker.C:
			_ = storage.CleanupUploadTemp(uploadMaxAge)

		case <-app.janitorStop:
			return
		}
	}
}

// serveHttp runs the HTTP accept loop until the server is shut down.
func (app *App) serveHttp(listener net.Listener) {

	var err error = app.httpServer.Serve(listener)

	if nil != err && http.ErrServerClosed != err {
		slog.Error("http server error", slog.String("error", err.Error()))
	}
}

// Shutdown stops all servers gracefully.
func (app *App) Shutdown() error {

	var ctx, cancel = context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Stop the background janitor.
	close(app.janitorStop)

	// Stop FTP first, then drain HTTP.
	if nil != app.ftpServer {

		var err error = app.ftpServer.Stop()
		if nil != err {
			slog.Warn("ftp stop error", slog.String("error", err.Error()))
		}
	}

	if nil != app.httpServer {

		var err error = app.httpServer.Shutdown(ctx)
		if nil != err {
			slog.Warn("http shutdown error", slog.String("error", err.Error()))
		}
	}

	if nil != app.store {

		var err error = app.store.Close()
		if nil != err {
			slog.Warn("store close error", slog.String("error", err.Error()))
		}
	}

	slog.Info("shutdown complete")

	return nil
}
