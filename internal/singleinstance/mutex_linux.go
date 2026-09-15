//go:build linux

package singleinstance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type Mutex struct {
	file *os.File
}

func (mutex *Mutex) Close() error {
	err := mutex.file.Close()
	if err != nil {
		return fmt.Errorf("close lock file: %w", err)
	}

	return nil
}

func Acquire(name string) (*Mutex, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find cache directory: %w", err)
	}

	lockDir := filepath.Join(cacheDir, "catlock")

	err = os.MkdirAll(lockDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	path := filepath.Join(lockDir, name+".lock")

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		file.Close()

		return nil, ErrAlreadyRunning
	}

	if err != nil {
		file.Close()

		return nil, fmt.Errorf("lock file: %w", err)
	}

	return &Mutex{file: file}, nil
}
