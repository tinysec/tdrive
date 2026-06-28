//go:build !windows

package app

// Run starts the application and blocks until termination. On non-Windows
// platforms this is always the signal-driven path (e.g. systemd sends SIGTERM).
func Run(application *App) error {

	return runWithSignals(application)
}
