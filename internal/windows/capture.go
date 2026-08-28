//go:build windows

package windows

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

type CaptureLog struct {
	events  chan string
	dropped uint64
}

func createCaptureFile(started time.Time) (*os.File, string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, "", fmt.Errorf("find local application-data directory: %w", err)
	}

	captureDir := filepath.Join(cacheDir, "CatLock", "Captures")

	err = os.MkdirAll(captureDir, 0o700)
	if err != nil {
		return nil, "", fmt.Errorf("create capture directory: %w", err)
	}

	name := started.Format("20060102-150405.000000000") + ".txt"
	capturePath := filepath.Join(captureDir, name)

	file, err := os.OpenFile(capturePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("create capture file: %w", err)
	}

	return file, capturePath, nil
}

func openCaptureFile(path string) error {
	pathPtr := utf16Ptr(path)

	result, _, _ := procShellExecuteW.Call(
		0,
		0, // Use the default "open" operation.
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		0,
		swShow,
	)

	if result <= 32 {
		return fmt.Errorf("open capture file: ShellExecuteW returned code %d", result)
	}

	return nil
}

func writeCapture(file *os.File, capture *CaptureLog, started time.Time) error {
	writer := bufio.NewWriterSize(file, 32*1024)

	_, err := fmt.Fprintf(
		writer,
		"CatLock capture\nStarted: %s\nSpecial keys are written as <KEY>.\n\n",
		started.Format(time.RFC3339),
	)

	if err != nil {
		return errors.Join(err, writer.Flush())
	}

	for text := range capture.events {
		_, err = writer.WriteString(text)
		if err != nil {
			return errors.Join(err, writer.Flush())
		}
	}

	_, err = fmt.Fprintf(
		writer,
		"\n\nEnded: %s\nDropped events: %d\n",
		time.Now().Format(time.RFC3339),
		capture.dropped,
	)

	if err != nil {
		return errors.Join(err, writer.Flush())
	}

	return writer.Flush()
}
