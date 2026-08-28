//go:build windows

package windows

import (
	"fmt"
	"strings"
	"unsafe"
)

func keyboardProc(code int, wParam, lParam uintptr) uintptr {
	app := activeApp
	if code < 0 || app == nil || app.hwnd == 0 {
		result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)

		return result
	}

	isDown := wParam == wmKeyDown || wParam == wmSysKeyDown
	isUp := wParam == wmKeyUp || wParam == wmSysKeyUp

	if !isDown && !isUp {
		return 1
	}

	//lint:ignore unsafeptr Windows passes KBDLLHOOKSTRUCT through this callback parameter.
	event := (*keyboardEvent)(unsafe.Pointer(lParam))
	virtualKey := event.virtualKey

	var wasDown bool

	if virtualKey < uint32(len(app.keysDown)) {
		wasDown = app.keysDown[virtualKey]
		app.keysDown[virtualKey] = isDown
	}

	app.updateModifiers()

	if isDown && virtualKey == vkF12 && app.ctrl && app.alt && app.shift {
		procPostMessageW.Call(app.hwnd, wmUnlock, 0, 0)

		return 1
	}

	if !isDown {
		return 1
	}

	if virtualKey == vkCapital && !wasDown {
		app.caps = !app.caps
	}

	if isModifier(virtualKey) && wasDown {
		return 1
	}

	text := app.keyText(virtualKey)

	app.keyCount++

	select {
	case app.capture.events <- text:
	default:
		// A hook must never wait on disk I/O: Windows removes low-level hooks
		// that take too long. The large buffer makes this unlikely, and the
		// transcript explicitly reports any dropped events.
		app.capture.dropped++
	}

	procPostMessageW.Call(app.hwnd, wmCapture, 0, 0)

	// A non-zero hook result prevents Windows from delivering this event.
	return 1
}

func (app *Application) updateModifiers() {
	app.shift = app.keyIsDown(vkShift) || app.keyIsDown(vkLShift) || app.keyIsDown(vkRShift)
	app.ctrl = app.keyIsDown(vkControl) || app.keyIsDown(vkLControl) || app.keyIsDown(vkRControl)
	app.alt = app.keyIsDown(vkMenu) || app.keyIsDown(vkLMenu) || app.keyIsDown(vkRMenu)
	app.win = app.keyIsDown(vkLWin) || app.keyIsDown(vkRWin)
}

func (app *Application) keyIsDown(virtualKey uint32) bool {
	return virtualKey < uint32(len(app.keysDown)) && app.keysDown[virtualKey]
}

func (app *Application) keyText(virtualKey uint32) string {
	if isModifier(virtualKey) {
		return "<" + keyName(virtualKey) + ">"
	}

	if app.ctrl || app.alt || app.win {
		var text strings.Builder

		text.WriteByte('<')

		if app.ctrl {
			text.WriteString("CTRL+")
		}

		if app.alt {
			text.WriteString("ALT+")
		}

		if app.win {
			text.WriteString("WIN+")
		}

		if app.shift {
			text.WriteString("SHIFT+")
		}

		text.WriteString(keyName(virtualKey))
		text.WriteByte('>')

		return text.String()
	}

	if virtualKey >= 'A' && virtualKey <= 'Z' {
		letter := byte(virtualKey)

		if app.shift == app.caps {
			letter += 'a' - 'A'
		}

		return string(letter)
	}

	if virtualKey >= '0' && virtualKey <= '9' {
		if app.shift {
			return ")!@#$%^&*("[virtualKey-'0' : virtualKey-'0'+1]
		}

		return string(byte(virtualKey))
	}

	if virtualKey >= vkNumpad0 && virtualKey <= vkNumpad0+9 {
		return string(byte('0' + virtualKey - vkNumpad0))
	}

	switch virtualKey {
	case vkSpace:
		return " "
	case vkTab:
		return "\t"
	case vkReturn:
		return "\n"
	case vkBack:
		return "<BACKSPACE>"
	case vkCapital:
		return "<CAPSLOCK>"
	case vkOEM1:
		return shifted(app.shift, ";", ":")
	case vkOEMPlus:
		return shifted(app.shift, "=", "+")
	case vkOEMComma:
		return shifted(app.shift, ",", "<")
	case vkOEMMinus:
		return shifted(app.shift, "-", "_")
	case vkOEMPeriod:
		return shifted(app.shift, ".", ">")
	case vkOEM2:
		return shifted(app.shift, "/", "?")
	case vkOEM3:
		return shifted(app.shift, "`", "~")
	case vkOEM4:
		return shifted(app.shift, "[", "{")
	case vkOEM5:
		return shifted(app.shift, `\`, "|")
	case vkOEM6:
		return shifted(app.shift, "]", "}")
	case vkOEM7:
		return shifted(app.shift, "'", `"`)
	case vkOEM102:
		return shifted(app.shift, `\`, "|")
	case vkMultiply:
		return "*"
	case vkAdd:
		return "+"
	case vkSubtract:
		return "-"
	case vkDecimal:
		return "."
	case vkDivide:
		return "/"
	default:
		return "<" + keyName(virtualKey) + ">"
	}
}

func shifted(shift bool, normal, shifted string) string {
	if shift {
		return shifted
	}

	return normal
}

func isModifier(virtualKey uint32) bool {
	switch virtualKey {
	case vkShift, vkLShift, vkRShift, vkControl, vkLControl, vkRControl, vkMenu, vkLMenu, vkRMenu, vkLWin, vkRWin:
		return true
	default:
		return false
	}
}

func keyName(virtualKey uint32) string {
	if virtualKey >= 'A' && virtualKey <= 'Z' {
		return string(byte(virtualKey))
	}

	if virtualKey >= '0' && virtualKey <= '9' {
		return string(byte(virtualKey))
	}

	if virtualKey >= vkF1 && virtualKey <= vkF24 {
		return fmt.Sprintf("F%d", virtualKey-vkF1+1)
	}

	switch virtualKey {
	case vkBack:
		return "BACKSPACE"
	case vkTab:
		return "TAB"
	case vkReturn:
		return "ENTER"
	case vkShift, vkLShift, vkRShift:
		return "SHIFT"
	case vkControl, vkLControl, vkRControl:
		return "CTRL"
	case vkMenu, vkLMenu, vkRMenu:
		return "ALT"
	case vkPause:
		return "PAUSE"
	case vkCapital:
		return "CAPSLOCK"
	case vkEscape:
		return "ESCAPE"
	case vkSpace:
		return "SPACE"
	case vkPrior:
		return "PAGE UP"
	case vkNext:
		return "PAGE DOWN"
	case vkEnd:
		return "END"
	case vkHome:
		return "HOME"
	case vkLeft:
		return "LEFT"
	case vkUp:
		return "UP"
	case vkRight:
		return "RIGHT"
	case vkDown:
		return "DOWN"
	case vkSnapshot:
		return "PRINT SCREEN"
	case vkInsert:
		return "INSERT"
	case vkDelete:
		return "DELETE"
	case vkLWin, vkRWin:
		return "WINDOWS"
	case vkNumLock:
		return "NUMLOCK"
	case vkScroll:
		return "SCROLLLOCK"
	default:
		return fmt.Sprintf("VK_%02X", virtualKey)
	}
}
