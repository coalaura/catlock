//go:build linux

package linux

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func (app *Application) runUI() error {
	if useWayland(os.Getenv("XDG_SESSION_TYPE"), os.Getenv("WAYLAND_DISPLAY")) {
		return app.runWayland()
	}

	return app.runX11()
}

func Run(version string) {
	err := run(version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "CatLock: %v\n", err)
	}
}

func run(version string) error {
	started := time.Now()

	file, capturePath, err := createCaptureFile(started)
	if err != nil {
		return err
	}

	capture := &CaptureLog{
		events: make(chan string, 8192),
	}

	writerDone := make(chan error, 1)

	go func() {
		writerDone <- writeCapture(file, capture, started)
	}()

	app := &Application{
		capture:     capture,
		capturePath: capturePath,
		version:     version,
	}

	uiErr := app.runUI()

	close(capture.events)

	writerErr := <-writerDone
	syncErr := file.Sync()
	closeErr := file.Close()

	err = errors.Join(uiErr, writerErr, syncErr, closeErr)
	if err != nil {
		return err
	}

	if app.openCaptureOnExit {
		return openCaptureFile(capturePath)
	}

	return nil
}

func useWayland(sessionType, display string) bool {
	return strings.EqualFold(sessionType, "wayland") || display != ""
}
