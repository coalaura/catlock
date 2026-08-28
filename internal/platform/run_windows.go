//go:build windows

package platform

import "github.com/coalaura/catlock/internal/windows"

func Run(version string) {
	windows.Run(version)
}
