package singleinstance

import "errors"

var ErrAlreadyRunning = errors.New("application is already running")
