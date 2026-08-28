//go:build linux

package platform

import "github.com/coalaura/catlock/internal/linux"

func Run(version string) {
	linux.Run(version)
}
