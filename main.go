package main

import (
	"errors"

	"github.com/coalaura/catlock/internal/platform"
	"github.com/coalaura/catlock/internal/singleinstance"
)

const mutexName = "com.coalaura.catlock"

var Version = "dev"

func main() {
	err := run()
	if err != nil {
		panic("CatLock: " + err.Error())
	}
}

func run() error {
	mutex, err := singleinstance.Acquire(mutexName)
	if err != nil {
		if errors.Is(err, singleinstance.ErrAlreadyRunning) {
			return nil
		}

		return err
	}

	defer mutex.Close()

	platform.Run(Version)

	return nil
}
