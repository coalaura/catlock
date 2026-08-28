package main

import "github.com/coalaura/catlock/internal/platform"

var version = "dev"

func main() {
	platform.Run(version)
}
