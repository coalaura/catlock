//go:build linux

package linux

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const testWaylandKeymap = `xkb_keymap {
	xkb_keycodes "test" {
		minimum = 8; maximum = 255;
		<AC01> = 38; <LFSH> = 50; <LCTL> = 37; <LALT> = 64; <FK12> = 96;
	};
	xkb_types "test" {
		type "ONE_LEVEL" { modifiers = None; map[None] = Level1; };
		type "FUNCTION" {
			modifiers = Shift+Control+Mod1;
			map[None] = Level1;
			map[Control+Mod1] = Level2;
			map[Shift+Control+Mod1] = Level2;
		};
	};
	xkb_compatibility "test" {};
	xkb_symbols "test" {
		key <AC01> { [ a ] };
		key <LFSH> { [ Shift_L ] };
		key <LCTL> { [ Control_L ] };
		key <LALT> { [ Alt_L ] };
		key <FK12> { type="FUNCTION", [ F12, XF86Switch_VT_12 ] };
		modifier_map Shift { <LFSH> };
		modifier_map Control { <LCTL> };
		modifier_map Mod1 { <LALT> };
	};
};` + "\x00"

type testWaylandScenario struct {
	name      string
	wantError string
	wantKeys  uint64
	wantLog   bool
}

type testWaylandGlobal struct {
	name    string
	version uint32
}

// This peer speaks the real wire protocol on a private test socket. No test
// connects to the user's compositor, maps a real window, or grabs real input.
type testWaylandCompositor struct {
	connection *net.UnixConn
	objects    map[uint32]string
	scenario   string
	keyboard   uint32
	pointer    uint32
	layer      uint32
	inhibitor  uint32
	surface    uint32
	width      uint32
	height     uint32
	configure  bool
	mapped     bool
	step       int
	compact    bool
	restored   bool
	exclusive  bool
	overlay    bool
	err        error
}

func (server *testWaylandCompositor) send(object, opcode uint32, payload []byte, descriptors ...int) {
	if server.err != nil {
		return
	}

	message := make([]byte, 0, len(payload)+8)
	message = appendTestWords(message, object, uint32(len(payload)+8)<<16|opcode)
	message = append(message, payload...)

	var rights []byte

	if len(descriptors) != 0 {
		rights = unix.UnixRights(descriptors...)
	}

	_, _, server.err = server.connection.WriteMsgUnix(message, rights, nil)
}

func (server *testWaylandCompositor) words(object, opcode uint32, values ...uint32) {
	server.send(object, opcode, appendTestWords(nil, values...))
}

func (server *testWaylandCompositor) run() error {
	defer server.connection.Close()

	server.err = server.connection.SetDeadline(time.Now().Add(10 * time.Second))

	var (
		buffer  [8192]byte
		control [1024]byte
	)

	queued := make([]byte, 0, len(buffer))

	for server.err == nil {
		count, controlCount, _, _, err := server.connection.ReadMsgUnix(buffer[:], control[:])
		// Immediate release may leave a queued key unread, making the Unix
		// stream report a reset instead of EOF. Both mean the lock owner left.
		if errors.Is(err, io.EOF) || errors.Is(err, unix.ECONNRESET) {
			return nil
		}

		if err != nil {
			return err
		}

		messages, err := unix.ParseSocketControlMessage(control[:controlCount])
		if err != nil {
			return err
		}

		for _, message := range messages {
			descriptors, parseErr := unix.ParseUnixRights(&message)
			if parseErr != nil {
				return parseErr
			}

			for _, descriptor := range descriptors {
				_ = unix.Close(descriptor)
			}
		}

		queued = append(queued, buffer[:count]...)

		for len(queued) >= 8 {
			header := testWaylandWord(queued, 1)

			size := int(header >> 16)
			if size < 8 {
				return fmt.Errorf("invalid request size %d", size)
			}

			if len(queued) < size {
				break
			}

			server.request(testWaylandWord(queued, 0), header&0xffff, queued[8:size])
			queued = queued[size:]
		}

		if len(queued) == 0 {
			queued = queued[:0]
		}
	}

	return server.err
}

func (server *testWaylandCompositor) request(object, opcode uint32, payload []byte) {
	class := server.objects[object]

	switch class {
	case "wl_display":
		identifier := testWaylandWord(payload, 0)

		if opcode == 0 {
			server.words(identifier, 0, 1)
			server.words(1, 1, identifier)
		} else {
			server.objects[identifier] = "wl_registry"
			server.advertise(identifier)
		}
	case "wl_registry":
		length := int(testWaylandWord(payload, 1))
		protocol := string(payload[8 : 8+length-1])
		identifier := binary.LittleEndian.Uint32(payload[12+(length+3)&^3:])
		server.objects[identifier] = protocol

		switch protocol {
		case "wl_seat":
			server.words(identifier, 0, 3)
		case "wl_output":
			server.words(identifier, 3, 2)
			server.words(identifier, 2)
		case "wl_shm":
			server.words(identifier, 0, 0)
			server.words(identifier, 0, 1)
		}
	case "wl_compositor":
		identifier := testWaylandWord(payload, 0)
		server.objects[identifier] = "wl_surface"

		if server.surface == 0 {
			server.surface = identifier
		}
	case "wl_shm":
		server.objects[testWaylandWord(payload, 0)] = "wl_shm_pool"
	case "wl_shm_pool":
		if opcode == 0 {
			server.objects[testWaylandWord(payload, 0)] = "wl_buffer"
		}
	case "wl_seat":
		identifier := testWaylandWord(payload, 0)

		switch opcode {
		case 1:
			server.keyboard = identifier
			server.objects[identifier] = "wl_keyboard"
			server.keymap()
		case 0:
			server.pointer = identifier
			server.objects[identifier] = "wl_pointer"
		}
	case "zwlr_layer_shell_v1":
		server.layer = testWaylandWord(payload, 0)
		server.objects[server.layer] = "zwlr_layer_surface_v1"
		server.overlay = testWaylandWord(payload, 3) == 3
	case "zwlr_layer_surface_v1":
		switch opcode {
		case 0:
			server.width = testWaylandWord(payload, 0)
			server.height = testWaylandWord(payload, 1)
			server.configure = true
		case 4:
			server.exclusive = testWaylandWord(payload, 0) == 1
		}
	case "zwp_keyboard_shortcuts_inhibit_manager_v1":
		server.inhibitor = testWaylandWord(payload, 0)
		server.objects[server.inhibitor] = "zwp_keyboard_shortcuts_inhibitor_v1"
	case "wl_surface":
		if opcode == 6 && object == server.surface {
			server.commit()
		}

		if opcode == 1 && testWaylandWord(payload, 0) != 0 {
			server.words(testWaylandWord(payload, 0), 0)
		}
	}
}

func (server *testWaylandCompositor) advertise(registry uint32) {
	globals := []testWaylandGlobal{
		{"wl_compositor", 4},
		{"wl_shm", 1},
		{"wl_seat", 5},
		{"wl_output", 2},
		{"zwlr_layer_shell_v1", 1},
		{"zwp_keyboard_shortcuts_inhibit_manager_v1", 1},
	}

	if server.scenario == "unsupported" {
		globals = globals[:4]
	}

	for index, global := range globals {
		payload := appendTestWords(nil, uint32(index+1), uint32(len(global.name)+1))
		payload = append(payload, global.name...)
		payload = append(payload, 0)

		for len(payload)%4 != 0 {
			payload = append(payload, 0)
		}

		payload = appendTestWords(payload, global.version)
		server.send(registry, 0, payload)
	}
}

func (server *testWaylandCompositor) keymap() {
	descriptor, err := unix.MemfdCreate("catlock-test-keymap", unix.MFD_CLOEXEC)
	if err != nil {
		server.err = err

		return
	}

	defer unix.Close(descriptor)

	_, err = unix.Write(descriptor, []byte(testWaylandKeymap))
	if err != nil {
		server.err = err

		return
	}

	server.send(server.keyboard, 0, appendTestWords(nil, 1, uint32(len(testWaylandKeymap))), descriptor)
}

func (server *testWaylandCompositor) click(horizontal, vertical uint32) {
	server.words(server.pointer, 0, 1, server.surface, horizontal*256, vertical*256)
	server.words(server.pointer, 3, 1, 1, 0x110, 0)
}

func (server *testWaylandCompositor) commit() {
	if server.configure {
		server.words(server.layer, 0, 1, server.width, server.height)
		server.configure = false

		return
	}

	if !server.mapped {
		server.mapped = true
		server.words(server.keyboard, 1, 1, server.surface, 0)
		server.words(server.keyboard, 4, 1, 0, 0, 0, 0)
		server.words(server.inhibitor, 0)

		return
	}

	if server.scenario != "compact and restore" {
		if server.step != 0 {
			return
		}

		server.step++
	}

	switch server.scenario {
	case "shortcut":
		// Send the chord and trailing key together: the latter must already be
		// queued when the client releases, rather than racing socket teardown.
		events := make([]byte, 0, 100)
		events = appendTestEvent(events, server.keyboard, 3, 1, 1, 30, 1)
		events = appendTestEvent(events, server.keyboard, 4, 1, 1|4|8, 0, 0, 0)
		events = appendTestEvent(events, server.keyboard, 3, 1, 1, 88, 1)
		events = appendTestEvent(events, server.keyboard, 3, 1, 1, 30, 1)
		_, server.err = server.connection.Write(events)
	case "focus loss":
		server.words(server.keyboard, 2, 1, server.surface)
	case "inhibitor loss":
		server.words(server.inhibitor, 1)
	case "disconnect":
		_ = server.connection.Close()
	case "compact and restore":
		switch server.step {
		case 0:
			server.click(660, 44)
		case 1:
			server.compact = server.width == compactWidth && server.height == compactHeight
			server.click(178, 30)
		case 2:
			server.restored = server.width == preferredWidth && server.height == preferredHeight
			server.click(600, 306)
		}

		server.step++
	case "release and log":
		server.click(400, 306)
	}
}

func TestWaylandBackendSelection(t *testing.T) {
	if !useWayland("wayland", "") || !useWayland("", "wayland-0") || useWayland("x11", "") {
		t.Fatal("backend selection must prefer native Wayland when the session or socket identifies it")
	}
}

func TestWaylandLockRequiresEveryKeyboard(t *testing.T) {
	first := new(byte)
	second := new(byte)
	app := &Application{lockPending: true}
	seat := &waylandSeat{keyboard: unsafe.Pointer(first), state: unsafe.Pointer(first), focused: true}
	session := &waylandSession{app: app, seats: map[unsafe.Pointer]*waylandSeat{unsafe.Pointer(first): seat}}
	session.updateLockState()

	if session.active || !app.lockPending {
		t.Fatal("focus alone must not claim the keyboard is locked")
	}

	seat.inhibited = true
	other := &waylandSeat{keyboard: unsafe.Pointer(second), state: unsafe.Pointer(second), inhibited: true}
	session.seats[unsafe.Pointer(second)] = other
	session.updateLockState()

	if session.active {
		t.Fatal("every keyboard seat must grant focus and inhibition")
	}

	other.focused = true
	session.updateLockState()

	if !session.active || app.lockPending {
		t.Fatal("confirmed keyboard lock should activate the UI")
	}

	other.inhibited = false
	session.updateLockState()

	if session.err == nil {
		t.Fatal("losing any seat's inhibition must end the lock")
	}
}

func TestWaylandProtocolLifecycle(t *testing.T) {
	scenarios := []testWaylandScenario{
		{name: "shortcut", wantKeys: 1},
		{name: "compact and restore"},
		{name: "release and log", wantLog: true},
		{name: "focus loss", wantError: "focus was lost"},
		{name: "inhibitor loss", wantError: "inhibition was withdrawn"},
		{name: "disconnect", wantError: "Wayland"},
		{name: "unsupported", wantError: "unsupported Wayland desktop"},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Keep the socket path short enough for sockaddr_un on every runner.
			socket := filepath.Join(t.TempDir(), "wl")

			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}

			defer listener.Close()
			t.Setenv("WAYLAND_DISPLAY", socket)
			t.Setenv("XDG_SESSION_TYPE", "wayland")
			server := &testWaylandCompositor{objects: map[uint32]string{1: "wl_display"}, scenario: scenario.name}
			finished := make(chan error, 1)

			go func() {
				connection, acceptErr := listener.AcceptUnix()
				if acceptErr != nil {
					finished <- acceptErr

					return
				}

				server.connection = connection
				finished <- server.run()
			}()

			capture := &CaptureLog{events: make(chan string, 16)}
			app := &Application{capture: capture, version: "test", capturePath: "/test/capture.txt"}
			err = app.runUI()
			_ = listener.Close()
			serverErr := <-finished

			if scenario.wantError == "" && err != nil {
				t.Fatalf("run native Wayland client: %v", err)
			}

			if scenario.wantError != "" && (err == nil || !strings.Contains(err.Error(), scenario.wantError)) {
				t.Fatalf("error = %v, want %q", err, scenario.wantError)
			}

			if serverErr != nil && scenario.name != "disconnect" {
				t.Fatalf("test compositor: %v", serverErr)
			}

			if app.keyCount != scenario.wantKeys || app.openCaptureOnExit != scenario.wantLog {
				t.Fatalf("keys = %d, open log = %v", app.keyCount, app.openCaptureOnExit)
			}

			if scenario.wantKeys != 0 {
				text := <-capture.events
				if text != "a" {
					t.Fatalf("captured text = %q, want a", text)
				}
			}

			if scenario.name != "unsupported" && (!server.exclusive || !server.overlay) {
				t.Fatal("keyboard lock did not request the exclusive overlay layer")
			}

			if scenario.name == "compact and restore" && (!server.compact || !server.restored) {
				t.Fatal("compact view did not resize and restore the same surface")
			}
		})
	}
}

func TestWaylandLockConfirmationTimeout(t *testing.T) {
	session := &waylandSession{app: &Application{lockPending: true}, deadline: time.Now().Add(-time.Second)}

	err := session.runEvents()
	if err == nil || !strings.Contains(err.Error(), "did not grant") {
		t.Fatalf("unconfirmed lock should time out, got %v", err)
	}

	if session.active || !session.app.lockPending {
		t.Fatal("timed-out lock must never activate the UI")
	}
}

func appendTestWords(destination []byte, values ...uint32) []byte {
	for _, value := range values {
		destination = binary.LittleEndian.AppendUint32(destination, value)
	}

	return destination
}

func appendTestEvent(destination []byte, object, opcode uint32, values ...uint32) []byte {
	size := uint32(8 + len(values)*4)
	destination = appendTestWords(destination, object, size<<16|opcode)

	return appendTestWords(destination, values...)
}

func testWaylandWord(payload []byte, index int) uint32 {
	return binary.LittleEndian.Uint32(payload[index*4:])
}
