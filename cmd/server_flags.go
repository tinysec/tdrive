package cmd

import (
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"

	"tdrive/internal/config"
)

const (
	defaultHttpPort int = 3000
	defaultFtpPort  int = 2121
)

// serverFlags is the small set of run options on the root command. There is no
// config file and no environment variables — these flags plus the built-in
// defaults are the entire configuration surface.
type serverFlags struct {
	host     string
	httpPort int
	ftpPort  int
	password string
	readOnly bool
}

// register attaches the flags to the command. The data root is taken as an
// optional positional argument (e.g. "tdrive /srv/share").
func (flags *serverFlags) register(command *cobra.Command) {

	command.Flags().StringVar(&flags.host, "host", "", "bind address for HTTP and FTP (default: all interfaces)")
	command.Flags().IntVar(&flags.httpPort, "port", defaultHttpPort, "HTTP port")
	command.Flags().IntVar(&flags.ftpPort, "ftp-port", defaultFtpPort, "FTP port; providing this enables the FTP server")
	command.Flags().StringVar(&flags.password, "password", "", "optional access password (default: no authentication)")
	command.Flags().BoolVar(&flags.readOnly, "readonly", false, "serve read-only: block all uploads, edits, and deletes")
}

// apply overlays the flag values onto a resolved config. HTTP is always enabled
// (the web UI is the core); FTP is enabled only when --ftp-port is given.
func (flags *serverFlags) apply(command *cobra.Command, args []string, cfg *config.Config) error {

	if len(args) >= 1 {
		cfg.Root = args[0]
	}

	// 1. HTTP listen address.
	var httpAddr string
	var err error

	httpAddr, err = listenAddr(flags.host, flags.httpPort)
	if nil != err {
		return fmt.Errorf("invalid --port: %w", err)
	}

	cfg.Http.Addr = httpAddr

	// 2. FTP listen address (enabled only when --ftp-port was provided).
	if command.Flags().Changed("ftp-port") {

		var ftpAddr string

		ftpAddr, err = listenAddr(flags.host, flags.ftpPort)
		if nil != err {
			return fmt.Errorf("invalid --ftp-port: %w", err)
		}

		cfg.Ftp.Enabled = true
		cfg.Ftp.Addr = ftpAddr

		// Advertise a concrete bind IP for FTP passive mode (helps on a LAN).
		if isConcreteIp(flags.host) {
			cfg.Ftp.PublicHost = flags.host
		}
	}

	// 3. Optional password and read-only mode.
	cfg.Password = flags.password
	cfg.ReadOnly = flags.readOnly

	return nil
}

// listenAddr builds a "host:port" listen address, validating the port range.
// An empty host means all interfaces (e.g. ":3000").
func listenAddr(host string, port int) (string, error) {

	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port %d out of range 1-65535", port)
	}

	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// isConcreteIp reports whether host is a specific (non-wildcard) IP literal.
func isConcreteIp(host string) bool {

	var ip net.IP = net.ParseIP(host)

	if nil == ip {
		return false
	}

	return false == ip.IsUnspecified()
}
