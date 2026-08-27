//go:build windows

package main

import (
	"errors"
	"runtime"
	"time"
)

var version = "dev"

func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := run()
	if err != nil {
		showError(err)
	}
}

func run() error {
	result, _, lastErr := procSetProcessDPIAware.Call()
	if result == 0 {
		return win32Error("SetProcessDPIAware", lastErr)
	}

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

	return openCaptureFile(capturePath)
}
