//go:build windows

package app

import (
	"golang.org/x/sys/windows/svc"

	"tdrive/internal/meta"
)

// Run starts the application and blocks until termination. When the process is
// launched by the Windows Service Control Manager it speaks the SCM protocol;
// otherwise (run from a console) it falls back to the normal signal path.
//
// There is no built-in install/uninstall: register the service yourself, e.g.
//
//	sc create tdrive binPath= "C:\tdrive.exe C:\data --port 3000" start= auto
func Run(application *App) error {

	var isService bool
	var err error

	isService, err = svc.IsWindowsService()
	if nil != err {
		return err
	}

	if false == isService {
		return runWithSignals(application)
	}

	return svc.Run(meta.Name, &windowsService{application: application})
}

// windowsService adapts the application to the Windows SCM handler interface.
type windowsService struct {
	application *App
}

// Execute implements svc.Handler: it reports status to the SCM and maps the Stop
// and Shutdown control requests to a graceful application shutdown.
func (service *windowsService) Execute(args []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {

	const accepted svc.Accepted = svc.AcceptStop | svc.AcceptShutdown

	// 1. Report that startup is in progress, then start the servers.
	status <- svc.Status{State: svc.StartPending}

	var err error = service.application.Start()
	if nil != err {
		return false, 1
	}

	status <- svc.Status{State: svc.Running, Accepts: accepted}

	// 2. Handle control requests until asked to stop.
	var request svc.ChangeRequest

	for request = range requests {

		switch request.Cmd {

		case svc.Interrogate:
			status <- request.CurrentStatus

		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}

			_ = service.application.Shutdown()

			return false, 0
		}
	}

	return false, 0
}
