//go:build linux

package linux

import (
	"bytes"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestMaskedComponentScalesIntoMask(t *testing.T) {
	actual := maskedComponent(0xff, 0x00ff0000)
	if actual != 0x00ff0000 {
		t.Fatalf("full red component = %#08x", actual)
	}

	actual = maskedComponent(0, 0x0000ff00)
	if actual != 0 {
		t.Fatalf("zero green component = %#08x", actual)
	}

	actual = maskedComponent(0xff, 0x0000f800)
	if actual != 0x0000f800 {
		t.Fatalf("full 5-bit component = %#08x", actual)
	}
}

func TestCompactViewHidesDetailsAndRestoresLayout(t *testing.T) {
	scales := [...]int{1, 2}

	for _, scale := range scales {
		renderer, err := newRenderer(scale)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(renderer.close)

		app := Application{
			screen:  &xproto.ScreenInfo{WidthInPixels: 1920, HeightInPixels: 1080},
			uiScale: scale,
		}

		app.refreshMetrics()

		regular, err := renderer.render(&app)
		if err != nil {
			t.Fatal(err)
		}

		regularX := app.windowX
		regularY := app.windowY

		releaseButton := app.button
		logButton := app.logButton

		app.compact = true
		app.refreshMetrics()

		compact, err := renderer.render(&app)
		if err != nil {
			t.Fatal(err)
		}

		if compact.Bounds().Dx() >= regular.Bounds().Dx() || compact.Bounds().Dy() >= regular.Bounds().Dy() {
			t.Fatalf("scale %d: compact bounds %v are not smaller than %v", scale, compact.Bounds(), regular.Bounds())
		}

		if app.windowX < 0 || app.windowY < 0 || app.windowX >= regularX || app.windowY >= regularY {
			t.Fatalf("scale %d: compact position (%d, %d) is not near the top-left", scale, app.windowX, app.windowY)
		}

		if app.button != (Rect{}) || app.logButton != (Rect{}) || app.toggleButton == (Rect{}) {
			t.Fatalf("scale %d: compact view must expose only the toggle button", scale)
		}

		app.capturePath = "/private/capture.txt"
		app.version = "hidden-version"

		hiddenDetails, err := renderer.render(&app)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(compact.Pix, hiddenDetails.Pix) {
			t.Fatalf("scale %d: compact view renders hidden details", scale)
		}

		app.keyCount++

		updated, err := renderer.render(&app)
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(compact.Pix, updated.Pix) {
			t.Fatalf("scale %d: compact counter did not update", scale)
		}

		app.compact = false
		app.refreshMetrics()

		restored, err := renderer.render(&app)
		if err != nil {
			t.Fatal(err)
		}

		if restored.Bounds() != regular.Bounds() || app.windowX != regularX || app.windowY != regularY {
			t.Fatalf("scale %d: regular window geometry was not restored", scale)
		}

		if app.button != releaseButton || app.logButton != logButton {
			t.Fatalf("scale %d: release buttons were not restored", scale)
		}
	}
}

func TestCompactViewIgnoresHiddenReleaseButtons(t *testing.T) {
	app := Application{
		compact:   true,
		uiScale:   1,
		button:    Rect{left: 10, top: 10, right: 30, bottom: 30},
		logButton: Rect{left: 40, top: 10, right: 60, bottom: 30},
	}

	positions := [...]int16{20, 50}

	for _, position := range positions {
		event := xproto.ButtonReleaseEvent{
			Detail: xproto.ButtonIndex1,
			EventX: position,
			EventY: 20,
		}

		repaint, done, err := app.handleEvent(event)
		if err != nil || repaint || done || app.openCaptureOnExit {
			t.Fatalf("hidden button at x=%d was activated: repaint=%v, done=%v, openCapture=%v, err=%v", position, repaint, done, app.openCaptureOnExit, err)
		}
	}
}
