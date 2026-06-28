package ftp

import (
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"log/slog"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"

	"tdrive/internal/config"
)

// errAuthFailed is returned when FTP credentials do not match.
var errAuthFailed error = errors.New("ftp authentication failed")

// errTLSDisabled is returned when a client requests TLS, which is not configured.
var errTLSDisabled error = errors.New("ftp TLS is not enabled")

// driver implements ftpserverlib's MainDriver over the shared sandboxed filesystem,
// so FTP clients browse exactly the same files as the web UI.
type driver struct {
	fs       afero.Fs
	settings *ftpserver.Settings
	user     string
	password string
}

func (d *driver) GetSettings() (*ftpserver.Settings, error) {

	return d.settings, nil
}

func (d *driver) ClientConnected(cc ftpserver.ClientContext) (string, error) {

	return "tdrive FTP", nil
}

func (d *driver) ClientDisconnected(cc ftpserver.ClientContext) {
}

// AuthUser authenticates a client. With no password configured access is open;
// otherwise the configured user and password must match.
func (d *driver) AuthUser(cc ftpserver.ClientContext, user string, pass string) (ftpserver.ClientDriver, error) {

	if "" == d.password {
		return d.fs, nil
	}

	var userMatch bool = user == d.user
	var passMatch bool = 1 == subtle.ConstantTimeCompare([]byte(pass), []byte(d.password))

	if userMatch && passMatch {
		return d.fs, nil
	}

	return nil, errAuthFailed
}

func (d *driver) GetTLSConfig() (*tls.Config, error) {

	return nil, errTLSDisabled
}

// Server wraps the FTP server with a non-blocking start and graceful stop so it
// can participate in the composite application lifecycle.
type Server struct {
	inner *ftpserver.FtpServer
	addr  string
}

// New constructs an FTP server bound to the shared filesystem and config.
func New(fs afero.Fs, cfg config.FtpConfig, password string) *Server {

	var settings ftpserver.Settings

	settings.ListenAddr = cfg.Addr
	settings.PublicHost = cfg.PublicHost
	settings.PassiveTransferPortRange = &ftpserver.PortRange{
		Start: cfg.PassivePortStart,
		End:   cfg.PassivePortEnd,
	}

	var d *driver = &driver{
		fs:       fs,
		settings: &settings,
		user:     cfg.User,
		password: password,
	}

	var server Server

	server.inner = ftpserver.NewFtpServer(d)
	server.addr = cfg.Addr

	return &server
}

// Start binds the listener and serves connections in the background.
func (server *Server) Start() error {

	var err error = server.inner.Listen()
	if nil != err {
		return err
	}

	slog.Info("ftp server listening", slog.String("addr", server.addr))

	go server.serve()

	return nil
}

// serve runs the accept loop until the listener is closed by Stop.
func (server *Server) serve() {

	var err error = server.inner.Serve()
	if nil != err {
		slog.Debug("ftp server stopped", slog.String("error", err.Error()))
	}
}

// Stop closes the listener and ends the accept loop.
func (server *Server) Stop() error {

	return server.inner.Stop()
}
