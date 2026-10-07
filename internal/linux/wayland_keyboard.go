//go:build linux

package linux

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/jezek/xgb/xproto"
	"golang.org/x/sys/unix"
)

const maxWaylandKeymapSize = 16 * 1024 * 1024

type waylandSeat struct {
	session    *waylandSession
	proxy      unsafe.Pointer
	keyboard   unsafe.Pointer
	pointer    unsafe.Pointer
	inhibitor  unsafe.Pointer
	keymap     unsafe.Pointer
	state      unsafe.Pointer
	focused    bool
	inhibited  bool
	inside     bool
	position   Point
	textBuffer [64]byte
}

func (seat *waylandSeat) event(opcode uint32, arguments []waylandArgument) {
	if opcode != 0 {
		return
	}

	capabilities := uint32(arguments[0])
	session := seat.session

	if capabilities&2 == 0 && seat.keyboard != nil {
		session.fail(fmt.Errorf("the Wayland keyboard was removed; keyboard released"))

		return
	}

	if capabilities&2 != 0 && seat.keyboard == nil {
		seat.keyboard = session.create(seat.proxy, 1, "wl_keyboard", session.native.version(seat.proxy), 0)

		session.listen(seat.keyboard, seat.keyboardEvent)

		seat.inhibit()
	}

	if capabilities&1 != 0 && seat.pointer == nil {
		seat.pointer = session.create(seat.proxy, 0, "wl_pointer", session.native.version(seat.proxy), 0)

		session.listen(seat.pointer, seat.pointerEvent)
	}

	if capabilities&1 == 0 {
		seat.inside = false
	}
}

func (seat *waylandSeat) inhibit() {
	session := seat.session
	if seat.keyboard == nil || session.surface == nil || seat.inhibitor != nil {
		return
	}

	seat.inhibitor = session.create(session.manager, 1, "zwp_keyboard_shortcuts_inhibitor_v1", 1, 0, waylandPointer(session.surface), waylandPointer(seat.proxy))

	session.listen(seat.inhibitor, func(opcode uint32, _ []waylandArgument) {
		seat.inhibited = opcode == 0
		if session.active && !seat.inhibited {
			session.fail(fmt.Errorf("keyboard shortcut inhibition was withdrawn; keyboard released"))
		}

		session.dirty = true
	})
}

func (seat *waylandSeat) closeKeymap() {
	if seat.state != nil {
		seat.session.native.xkbStateUnref(seat.state)
		seat.state = nil
	}

	if seat.keymap != nil {
		seat.session.native.xkbKeymapUnref(seat.keymap)
		seat.keymap = nil
	}
}

func (seat *waylandSeat) loadKeymap(format uint32, descriptor int, size uint32) error {
	defer unix.Close(descriptor)

	if seat.session.err != nil || seat.session.done {
		return nil
	}

	if format != 1 || size == 0 || size > maxWaylandKeymapSize {
		return fmt.Errorf("the Wayland desktop supplied an unsupported keyboard map")
	}

	var info unix.Stat_t

	err := unix.Fstat(descriptor, &info)
	if err != nil {
		return fmt.Errorf("inspect Wayland keyboard map: %w", err)
	}

	if info.Size < int64(size) {
		return fmt.Errorf("the Wayland desktop supplied a truncated keyboard map")
	}

	data, err := unix.Mmap(descriptor, 0, int(size), unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		return fmt.Errorf("map Wayland keyboard layout: %w", err)
	}

	defer unix.Munmap(data)

	if data[len(data)-1] != 0 {
		return fmt.Errorf("the Wayland keyboard map is missing its terminator")
	}

	native := seat.session.native

	keymap := native.xkbKeymapNew(seat.session.xkbContext, &data[0], 1, 0)
	if keymap == nil {
		return fmt.Errorf("parse Wayland keyboard layout")
	}

	state := native.xkbStateNew(keymap)
	if state == nil {
		native.xkbKeymapUnref(keymap)

		return fmt.Errorf("create Wayland keyboard state")
	}

	seat.closeKeymap()
	seat.keymap = keymap
	seat.state = state

	return nil
}

func (seat *waylandSeat) keyboardEvent(opcode uint32, arguments []waylandArgument) {
	session := seat.session

	switch opcode {
	case 0:
		err := seat.loadKeymap(uint32(arguments[0]), int(int32(arguments[1])), uint32(arguments[2]))
		if err != nil {
			session.fail(err)
		}
	case 1:
		seat.focused = arguments[1].pointer() == session.surface
	case 2:
		seat.focused = false

		if session.active {
			session.fail(fmt.Errorf("keyboard focus was lost; keyboard released"))
		}
	case 3:
		if uint32(arguments[3]) == 1 && seat.focused && seat.state != nil && !session.done && session.err == nil {
			if seat.captureKey(uint32(arguments[2]) + 8) {
				session.done = true
			}

			session.dirty = true
		}
	case 4:
		if seat.state != nil {
			session.native.xkbUpdateMask(seat.state, uint32(arguments[1]), uint32(arguments[2]), uint32(arguments[3]), 0, 0, uint32(arguments[4]))
		}
	}
}

func (seat *waylandSeat) captureKey(keycode uint32) bool {
	native := seat.session.native
	keysym := xproto.Keysym(native.xkbKeySym(seat.state, keycode))
	layout := native.xkbKeyLayout(seat.state, keycode)

	var symbols *uint32

	base := keysym

	count := native.xkbKeySymbols(seat.keymap, keycode, layout, 0, &symbols)
	if count > 0 {
		base = xproto.Keysym(*symbols)
	}

	ctrl := native.xkbModActive(seat.state, "Control\x00", 8) > 0
	shift := native.xkbModActive(seat.state, "Shift\x00", 8) > 0
	alt := native.xkbModActive(seat.state, "Mod1\x00", 8) > 0
	super := native.xkbModActive(seat.state, "Mod4\x00", 8) > 0

	// Some layouts translate Ctrl+Alt+F12 to an XF86 virtual-terminal symbol.
	// Match the unmodified key so the release chord remains usable there too.
	if base == keysymF12 && ctrl && shift && alt {
		return true
	}

	length := native.xkbKeyUTF8(seat.state, keycode, &seat.textBuffer[0], uintptr(len(seat.textBuffer)))
	text := ""

	if length > 0 && int(length) < len(seat.textBuffer) {
		text = string(seat.textBuffer[:length])
	}

	if ctrl || alt || super {
		keysym = base
	}

	text = waylandKeyText(keysym, text, ctrl, shift, alt, super)
	app := seat.session.app
	app.keyCount++

	select {
	case app.capture.events <- text:
	default:
		app.capture.dropped++
	}

	return false
}

func (seat *waylandSeat) pointerEvent(opcode uint32, arguments []waylandArgument) {
	session := seat.session

	switch opcode {
	case 0:
		seat.inside = arguments[1].pointer() == session.surface
		seat.position = waylandPosition(arguments[2], arguments[3])

		if seat.inside {
			session.request(seat.pointer, 0, arguments[0], waylandPointer(session.cursor), 0, 0)
		}
	case 1:
		seat.inside = false
	case 2:
		seat.position = waylandPosition(arguments[1], arguments[2])
	case 3:
		if seat.inside && uint32(arguments[2]) == 0x110 && uint32(arguments[3]) == 0 {
			seat.click()
		}
	}
}

func (seat *waylandSeat) click() {
	session := seat.session

	app := session.app
	if app.toggleButton.contains(seat.position) && !app.lockPending {
		app.compact = !app.compact
		session.setLayout()
		session.request(session.surface, 6)

		return
	}

	if !app.compact && (app.button.contains(seat.position) || app.logButton.contains(seat.position)) {
		app.openCaptureOnExit = app.logButton.contains(seat.position)
		session.done = true
	}
}

func waylandKeyText(keysym xproto.Keysym, text string, ctrl, shift, alt, super bool) string {
	if isModifierKeysym(keysym) {
		return "<" + keysymName(keysym) + ">"
	}

	if ctrl || alt || super {
		var result strings.Builder

		result.Grow(48)
		result.WriteByte('<')

		if ctrl {
			result.WriteString("CTRL+")
		}

		if alt {
			result.WriteString("ALT+")
		}

		if super {
			result.WriteString("WIN+")
		}

		if shift {
			result.WriteString("SHIFT+")
		}

		result.WriteString(keysymName(keysym))
		result.WriteByte('>')

		return result.String()
	}

	switch keysym {
	case keysymReturn, keysymKeypadEnter:
		return "\n"
	case keysymTab:
		return "\t"
	case keysymBackspace:
		return "<BACKSPACE>"
	}

	if text != "" && text[0] >= 0x20 && text[0] != 0x7f {
		return text
	}

	return "<" + keysymName(keysym) + ">"
}

func waylandPosition(horizontal, vertical waylandArgument) Point {
	return Point{x: int(int32(horizontal)) >> 8, y: int(int32(vertical)) >> 8}
}
