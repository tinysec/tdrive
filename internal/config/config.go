package config

import (
	"fmt"
	"strings"

	"tdrive/internal/meta"
)

// HttpConfig controls the web (HTTP) access surface.
type HttpConfig struct {
	Enabled bool
	Addr    string
}

// FtpConfig controls the optional FTP access surface that serves the same files.
type FtpConfig struct {
	Enabled bool

	Addr string

	// User is the FTP login name. When Password is empty, any user/password is accepted.
	User string

	// PassivePortStart and PassivePortEnd bound the passive-mode data port range.
	PassivePortStart int
	PassivePortEnd   int

	// PublicHost is the address advertised to clients for passive connections.
	// Empty lets the FTP library derive it from the control connection, which is
	// correct for the LAN use case this tool targets.
	PublicHost string
}

// Config is the fully-resolved runtime configuration for the disk server.
//
// There is no config file and no environment variables: settings come from CLI
// flags (applied by the command layer) on top of the built-in defaults. The
// struct is just the resolved result handed to the application.
type Config struct {
	// Root is the directory whose contents are served over HTTP and FTP.
	Root string

	// Password, when non-empty, protects every access path. Empty means open access.
	Password string

	// ReadOnly, when true, blocks every write across all surfaces (HTTP, REST,
	// WebDAV, FTP). It is independent of Password: the password controls who may
	// access, ReadOnly controls whether they may change anything.
	ReadOnly bool

	// Language is the default UI language ("zh" or "en"); users can switch in the UI.
	Language string

	Http HttpConfig
	Ftp  FtpConfig
}

// New returns a Config populated with the built-in defaults. The command layer
// applies CLI flags on top of this, then calls Validate.
func New() *Config {

	var config Config = defaults()

	return &config
}

// defaults returns the batteries-included default configuration.
func defaults() Config {

	var config Config

	config.Root = "."
	config.Password = ""
	config.ReadOnly = false
	config.Language = "zh"

	config.Http.Enabled = true
	config.Http.Addr = ":3000"

	config.Ftp.Enabled = false
	config.Ftp.Addr = ":2121"
	config.Ftp.User = meta.Name
	config.Ftp.PassivePortStart = 30000
	config.Ftp.PassivePortEnd = 30009
	config.Ftp.PublicHost = ""

	return config
}

// Validate normalizes derived values and checks required invariants. The command
// layer calls this after applying CLI flags.
func (config *Config) Validate() error {

	config.Language = strings.ToLower(strings.TrimSpace(config.Language))

	if "zh" != config.Language && "en" != config.Language {
		config.Language = "zh"
	}

	if "" == strings.TrimSpace(config.Root) {
		return fmt.Errorf("data root directory must not be empty")
	}

	if false == config.Http.Enabled && false == config.Ftp.Enabled {
		return fmt.Errorf("at least one of http or ftp must be enabled")
	}

	if config.Ftp.Enabled {

		if config.Ftp.PassivePortStart <= 0 || config.Ftp.PassivePortEnd < config.Ftp.PassivePortStart {
			return fmt.Errorf("ftp passive port range is invalid: %d-%d", config.Ftp.PassivePortStart, config.Ftp.PassivePortEnd)
		}
	}

	return nil
}

// AuthEnabled reports whether a password gate is configured.
func (config *Config) AuthEnabled() bool {

	return "" != config.Password
}
