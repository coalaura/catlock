//go:build windows

package singleinstance

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

type Mutex struct {
	handle windows.Handle
}

func (mutex *Mutex) Close() error {
	err := windows.CloseHandle(mutex.handle)
	if err != nil {
		return fmt.Errorf("close mutex handle: %w", err)
	}

	return nil
}

func Acquire(name string) (*Mutex, error) {
	namePtr, err := windows.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return nil, fmt.Errorf("encode mutex name: %w", err)
	}

	handle, err := windows.CreateMutex(nil, false, namePtr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}

		return nil, ErrAlreadyRunning
	}

	if err != nil {
		return nil, fmt.Errorf("create mutex: %w", err)
	}

	return &Mutex{handle: handle}, nil
}
