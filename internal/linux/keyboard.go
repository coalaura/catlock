//go:build linux

package linux

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	keysymBackspace   xproto.Keysym = 0xff08
	keysymTab         xproto.Keysym = 0xff09
	keysymReturn      xproto.Keysym = 0xff0d
	keysymPause       xproto.Keysym = 0xff13
	keysymScrollLock  xproto.Keysym = 0xff14
	keysymEscape      xproto.Keysym = 0xff1b
	keysymHome        xproto.Keysym = 0xff50
	keysymLeft        xproto.Keysym = 0xff51
	keysymUp          xproto.Keysym = 0xff52
	keysymRight       xproto.Keysym = 0xff53
	keysymDown        xproto.Keysym = 0xff54
	keysymPageUp      xproto.Keysym = 0xff55
	keysymPageDown    xproto.Keysym = 0xff56
	keysymEnd         xproto.Keysym = 0xff57
	keysymPrint       xproto.Keysym = 0xff61
	keysymInsert      xproto.Keysym = 0xff63
	keysymNumLock     xproto.Keysym = 0xff7f
	keysymKeypadEnter xproto.Keysym = 0xff8d
	keysymKeypadF1    xproto.Keysym = 0xff91
	keysymKeypad0     xproto.Keysym = 0xffb0
	keysymKeypad9     xproto.Keysym = 0xffb9
	keysymKeypadMul   xproto.Keysym = 0xffaa
	keysymKeypadAdd   xproto.Keysym = 0xffab
	keysymKeypadSub   xproto.Keysym = 0xffad
	keysymKeypadDec   xproto.Keysym = 0xffae
	keysymKeypadDiv   xproto.Keysym = 0xffaf
	keysymF1          xproto.Keysym = 0xffbe
	keysymF12         xproto.Keysym = 0xffc9
	keysymF35         xproto.Keysym = 0xffe0
	keysymShiftLeft   xproto.Keysym = 0xffe1
	keysymShiftRight  xproto.Keysym = 0xffe2
	keysymCtrlLeft    xproto.Keysym = 0xffe3
	keysymCtrlRight   xproto.Keysym = 0xffe4
	keysymCapsLock    xproto.Keysym = 0xffe5
	keysymAltLeft     xproto.Keysym = 0xffe9
	keysymAltRight    xproto.Keysym = 0xffea
	keysymSuperLeft   xproto.Keysym = 0xffeb
	keysymSuperRight  xproto.Keysym = 0xffec
	keysymDelete      xproto.Keysym = 0xffff
	keysymModeSwitch  xproto.Keysym = 0xff7e
	keysymLevel3Shift xproto.Keysym = 0xfe03
)

type keyboardMapping struct {
	firstKeycode   xproto.Keycode
	lastKeycode    xproto.Keycode
	keysymsPerCode int
	keysyms        []xproto.Keysym
	altMask        uint16
	superMask      uint16
	numLockMask    uint16
	level3Mask     uint16
}

func loadKeyboardMapping(connection *xgb.Conn) (*keyboardMapping, error) {
	setup := xproto.Setup(connection)
	keycodeCount := byte(int(setup.MaxKeycode) - int(setup.MinKeycode) + 1)

	keyboardReply, err := xproto.GetKeyboardMapping(connection, setup.MinKeycode, keycodeCount).Reply()
	if err != nil {
		return nil, fmt.Errorf("read X11 keyboard mapping: %w", err)
	}

	modifierReply, err := xproto.GetModifierMapping(connection).Reply()
	if err != nil {
		return nil, fmt.Errorf("read X11 modifier mapping: %w", err)
	}

	mapping := &keyboardMapping{
		firstKeycode:   setup.MinKeycode,
		lastKeycode:    setup.MaxKeycode,
		keysymsPerCode: int(keyboardReply.KeysymsPerKeycode),
		keysyms:        keyboardReply.Keysyms,
	}

	mapping.loadModifierMasks(modifierReply)

	return mapping, nil
}

func (mapping *keyboardMapping) loadModifierMasks(reply *xproto.GetModifierMappingReply) {
	perModifier := int(reply.KeycodesPerModifier)

	for modifierIndex := range 8 {
		mask := uint16(1 << modifierIndex)
		start := modifierIndex * perModifier

		for index := start; index < start+perModifier; index++ {
			keycode := reply.Keycodes[index]

			for _, keysym := range mapping.symbols(keycode) {
				switch keysym {
				case keysymAltLeft, keysymAltRight:
					mapping.altMask |= mask
				case keysymSuperLeft, keysymSuperRight:
					mapping.superMask |= mask
				case keysymNumLock:
					mapping.numLockMask |= mask
				case keysymModeSwitch, keysymLevel3Shift:
					mapping.level3Mask |= mask
				}
			}
		}
	}

	if mapping.altMask == 0 {
		mapping.altMask = xproto.ModMask1
	}

	if mapping.superMask == 0 {
		mapping.superMask = xproto.ModMask4
	}
}

func (mapping *keyboardMapping) symbols(keycode xproto.Keycode) []xproto.Keysym {
	if keycode < mapping.firstKeycode || keycode > mapping.lastKeycode || mapping.keysymsPerCode == 0 {
		return nil
	}

	start := (int(keycode) - int(mapping.firstKeycode)) * mapping.keysymsPerCode
	end := start + mapping.keysymsPerCode
	if start < 0 || end > len(mapping.keysyms) {
		return nil
	}

	return mapping.keysyms[start:end]
}

func (mapping *keyboardMapping) baseKeysym(keycode xproto.Keycode) xproto.Keysym {
	keysyms := mapping.symbols(keycode)
	if len(keysyms) == 0 {
		return 0
	}

	return keysyms[0]
}

func (mapping *keyboardMapping) selectedKeysym(keycode xproto.Keycode, state uint16) xproto.Keysym {
	keysyms := mapping.symbols(keycode)
	if len(keysyms) == 0 {
		return 0
	}

	level := 0

	if state&mapping.level3Mask != 0 && len(keysyms) > 2 {
		level = 2
	}

	base := keysyms[level]
	if base == 0 {
		level = 0
		base = keysyms[0]
	}

	shifted := base

	if len(keysyms) > level+1 && keysyms[level+1] != 0 {
		shifted = keysyms[level+1]
	}

	baseIsKeypadDigit := base >= keysymKeypad0 && base <= keysymKeypad9
	shiftedIsKeypadDigit := shifted >= keysymKeypad0 && shifted <= keysymKeypad9

	if baseIsKeypadDigit != shiftedIsKeypadDigit {
		wantDigit := state&mapping.numLockMask != 0

		if state&xproto.ModMaskShift != 0 {
			wantDigit = !wantDigit
		}

		if baseIsKeypadDigit == wantDigit {
			return base
		}

		return shifted
	}

	baseRune, basePrintable := keysymRune(base)
	shiftedRune, shiftedPrintable := keysymRune(shifted)
	isLetter := basePrintable && unicode.IsLetter(baseRune)

	if isLetter && shifted == base {
		shiftedRune = unicode.ToUpper(baseRune)
		shifted = runeKeysym(shiftedRune)
		shiftedPrintable = true
	}

	if isLetter && shiftedPrintable && unicode.ToUpper(baseRune) == unicode.ToUpper(shiftedRune) {
		shift := state&xproto.ModMaskShift != 0
		capsLock := state&xproto.ModMaskLock != 0

		if shift != capsLock {
			return shifted
		}

		return base
	}

	if state&xproto.ModMaskShift != 0 {
		return shifted
	}

	return base
}

func (app *Application) captureKey(event xproto.KeyPressEvent) bool {
	base := app.keyboard.baseKeysym(event.Detail)

	ctrl := event.State&xproto.ModMaskControl != 0
	shift := event.State&xproto.ModMaskShift != 0
	alt := event.State&app.keyboard.altMask != 0

	if base == keysymF12 && ctrl && shift && alt {
		return true
	}

	text := app.keyboard.keyText(event.Detail, event.State)
	app.keyCount++

	select {
	case app.capture.events <- text:
	default:
		app.capture.dropped++
	}

	return false
}

func (mapping *keyboardMapping) keyText(keycode xproto.Keycode, state uint16) string {
	base := mapping.baseKeysym(keycode)
	if isModifierKeysym(base) {
		return "<" + keysymName(base) + ">"
	}

	ctrl := state&xproto.ModMaskControl != 0
	alt := state&mapping.altMask != 0
	super := state&mapping.superMask != 0
	shift := state&xproto.ModMaskShift != 0

	if ctrl || alt || super {
		var text strings.Builder

		text.WriteByte('<')

		if ctrl {
			text.WriteString("CTRL+")
		}

		if alt {
			text.WriteString("ALT+")
		}

		if super {
			text.WriteString("WIN+")
		}

		if shift {
			text.WriteString("SHIFT+")
		}

		text.WriteString(keysymName(base))
		text.WriteByte('>')

		return text.String()
	}

	selected := mapping.selectedKeysym(keycode, state)

	switch selected {
	case keysymTab:
		return "\t"
	case keysymReturn, keysymKeypadEnter:
		return "\n"
	case keysymBackspace:
		return "<BACKSPACE>"
	case keysymCapsLock:
		return "<CAPSLOCK>"
	case keysymKeypadMul:
		return "*"
	case keysymKeypadAdd:
		return "+"
	case keysymKeypadSub:
		return "-"
	case keysymKeypadDec:
		return "."
	case keysymKeypadDiv:
		return "/"
	}

	if selected >= keysymKeypad0 && selected <= keysymKeypad9 {
		return string(rune('0' + selected - keysymKeypad0))
	}

	textRune, printable := keysymRune(selected)
	if printable && !unicode.IsControl(textRune) {
		return string(textRune)
	}

	return "<" + keysymName(selected) + ">"
}

func isModifierKeysym(keysym xproto.Keysym) bool {
	switch keysym {
	case keysymShiftLeft, keysymShiftRight,
		keysymCtrlLeft, keysymCtrlRight,
		keysymAltLeft, keysymAltRight,
		keysymSuperLeft, keysymSuperRight,
		keysymModeSwitch, keysymLevel3Shift:
		return true
	default:
		return false
	}
}

func keysymRune(keysym xproto.Keysym) (rune, bool) {
	if keysym >= 0x20 && keysym <= 0x7e || keysym >= 0xa0 && keysym <= 0xff {
		return rune(keysym), true
	}

	if keysym&0xff000000 == 0x01000000 {
		value := rune(keysym & 0x00ffffff)

		if unicode.IsGraphic(value) || unicode.IsSpace(value) {
			return value, true
		}
	}

	return 0, false
}

func runeKeysym(value rune) xproto.Keysym {
	if value <= 0xff {
		return xproto.Keysym(value)
	}

	return xproto.Keysym(0x01000000 | value)
}

func keysymName(keysym xproto.Keysym) string {
	textRune, printable := keysymRune(keysym)
	if printable {
		return strings.ToUpper(string(textRune))
	}

	if keysym >= keysymF1 && keysym <= keysymF35 {
		return fmt.Sprintf("F%d", keysym-keysymF1+1)
	}

	if keysym >= keysymKeypad0 && keysym <= keysymKeypad9 {
		return fmt.Sprintf("NUMPAD%d", keysym-keysymKeypad0)
	}

	switch keysym {
	case keysymBackspace:
		return "BACKSPACE"
	case keysymTab:
		return "TAB"
	case keysymReturn, keysymKeypadEnter:
		return "ENTER"
	case keysymShiftLeft, keysymShiftRight:
		return "SHIFT"
	case keysymCtrlLeft, keysymCtrlRight:
		return "CTRL"
	case keysymAltLeft, keysymAltRight:
		return "ALT"
	case keysymSuperLeft, keysymSuperRight:
		return "WINDOWS"
	case keysymModeSwitch, keysymLevel3Shift:
		return "ALTGR"
	case keysymPause:
		return "PAUSE"
	case keysymCapsLock:
		return "CAPSLOCK"
	case keysymEscape:
		return "ESCAPE"
	case keysymHome:
		return "HOME"
	case keysymLeft:
		return "LEFT"
	case keysymUp:
		return "UP"
	case keysymRight:
		return "RIGHT"
	case keysymDown:
		return "DOWN"
	case keysymPageUp:
		return "PAGE UP"
	case keysymPageDown:
		return "PAGE DOWN"
	case keysymEnd:
		return "END"
	case keysymPrint:
		return "PRINT SCREEN"
	case keysymInsert:
		return "INSERT"
	case keysymDelete:
		return "DELETE"
	case keysymNumLock:
		return "NUMLOCK"
	case keysymScrollLock:
		return "SCROLLLOCK"
	case keysymKeypadMul:
		return "MULTIPLY"
	case keysymKeypadAdd:
		return "ADD"
	case keysymKeypadSub:
		return "SUBTRACT"
	case keysymKeypadDec:
		return "DECIMAL"
	case keysymKeypadDiv:
		return "DIVIDE"
	case 0:
		return "UNKNOWN"
	default:
		return fmt.Sprintf("KEYSYM_%08X", keysym)
	}
}
