//go:build linux

package linux

import (
	"math"
	"strconv"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	baseDPI            = 96.0
	referenceShortSide = 1200.0
	maxUIScale         = 4
)

func detectUIScale(connection *xgb.Conn, screen *xproto.ScreenInfo) int {
	dpi := 0.0

	value, known := readXftDpi(connection, screen.Root)
	if known {
		dpi = value
	}

	shortSide := min(int(screen.WidthInPixels), int(screen.HeightInPixels))

	return resolveUIScale(shortSide, dpi)
}

func resolveUIScale(shortSide int, dpi float64) int {
	dpiScale := int(math.Round(dpi / baseDPI))
	screenScale := int(math.Round(float64(shortSide) / referenceShortSide))

	scale := max(dpiScale, screenScale, 1)

	return min(scale, maxUIScale)
}

func readXftDpi(connection *xgb.Conn, root xproto.Window) (float64, bool) {
	atom, err := xproto.InternAtom(connection, false, uint16(len("RESOURCE_MANAGER")), "RESOURCE_MANAGER").Reply()
	if err != nil || atom == nil {
		return 0, false
	}

	property, err := xproto.GetProperty(
		connection,
		false,
		root,
		atom.Atom,
		xproto.AtomString,
		0,
		65536,
	).Reply()

	if err != nil || property == nil {
		return 0, false
	}

	return parseXftDpi(string(property.Value))
}

func parseXftDpi(resources string) (float64, bool) {
	for line := range strings.Lines(resources) {
		name, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) != "Xft.dpi" {
			continue
		}

		dpi, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || dpi <= 0 {
			continue
		}

		return dpi, true
	}

	return 0, false
}
