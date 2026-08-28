//go:build linux

package linux

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

const (
	testKeycode   xproto.Keycode = 8
	testEuro      xproto.Keysym  = 0x010020ac
	testKeypadEnd xproto.Keysym  = 0xff9c
)

func TestSelectedKeysymUsesShiftAndCapsLock(t *testing.T) {
	mapping := testKeyboardMapping('a', 'A', 0, 0)

	actual := mapping.selectedKeysym(testKeycode, 0)
	if actual != 'a' {
		t.Fatalf("unmodified keysym = %#x, want a", actual)
	}

	actual = mapping.selectedKeysym(testKeycode, xproto.ModMaskShift)
	if actual != 'A' {
		t.Fatalf("shifted keysym = %#x, want A", actual)
	}

	actual = mapping.selectedKeysym(testKeycode, xproto.ModMaskLock)
	if actual != 'A' {
		t.Fatalf("caps-lock keysym = %#x, want A", actual)
	}

	state := uint16(xproto.ModMaskShift | xproto.ModMaskLock)
	actual = mapping.selectedKeysym(testKeycode, state)
	if actual != 'a' {
		t.Fatalf("shifted caps-lock keysym = %#x, want a", actual)
	}
}

func TestSelectedKeysymUsesAltGrLevel(t *testing.T) {
	mapping := testKeyboardMapping('e', 'E', testEuro, testEuro)
	mapping.level3Mask = xproto.ModMask5

	actual := mapping.selectedKeysym(testKeycode, xproto.ModMask5)
	if actual != testEuro {
		t.Fatalf("AltGr keysym = %#x, want %#x", actual, testEuro)
	}

	actualText := mapping.keyText(testKeycode, xproto.ModMask5)
	if actualText != "€" {
		t.Fatalf("AltGr text = %q, want euro sign", actualText)
	}
}

func TestSelectedKeysymUsesNumLock(t *testing.T) {
	mapping := testKeyboardMapping(testKeypadEnd, keysymKeypad0+1, 0, 0)
	mapping.numLockMask = xproto.ModMask2

	actual := mapping.selectedKeysym(testKeycode, 0)
	if actual != testKeypadEnd {
		t.Fatalf("keypad keysym = %#x, want keypad end", actual)
	}

	actualText := mapping.keyText(testKeycode, xproto.ModMask2)
	if actualText != "1" {
		t.Fatalf("num-lock text = %q, want 1", actualText)
	}

	state := uint16(xproto.ModMask2 | xproto.ModMaskShift)
	actual = mapping.selectedKeysym(testKeycode, state)
	if actual != testKeypadEnd {
		t.Fatalf("shifted num-lock keysym = %#x, want keypad end", actual)
	}
}

func TestCaptureKeyRecognizesEmergencyRelease(t *testing.T) {
	mapping := testKeyboardMapping(keysymF12, keysymF12, 0, 0)
	mapping.altMask = xproto.ModMask1

	app := Application{
		capture:  &CaptureLog{events: make(chan string, 1)},
		keyboard: mapping,
	}

	event := xproto.KeyPressEvent{
		Detail: testKeycode,
		State:  xproto.ModMaskControl | xproto.ModMaskShift | xproto.ModMask1,
	}

	if !app.captureKey(event) {
		t.Fatal("emergency shortcut did not release")
	}

	if app.keyCount != 0 {
		t.Fatalf("emergency shortcut incremented key count to %d", app.keyCount)
	}
}

func TestKeyTextFormatsModifiedKey(t *testing.T) {
	mapping := testKeyboardMapping('a', 'A', 0, 0)
	state := uint16(xproto.ModMaskControl | xproto.ModMaskShift)

	actual := mapping.keyText(testKeycode, state)
	if actual != "<CTRL+SHIFT+A>" {
		t.Fatalf("modified key text = %q", actual)
	}
}

func testKeyboardMapping(first, second, third, fourth xproto.Keysym) *keyboardMapping {
	return &keyboardMapping{
		firstKeycode:   testKeycode,
		lastKeycode:    testKeycode,
		keysymsPerCode: 4,
		keysyms:        []xproto.Keysym{first, second, third, fourth},
	}
}
