//go:build windows

package windows

import (
	"errors"
	"runtime"
	"time"
)

func Run(version string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := run(version)
	if err != nil {
		showError(err)
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

	activeApp = app

	uiErr := app.runUI()

	activeApp = nil
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
