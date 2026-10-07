//go:build linux

package linux

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

const waylandStartupTimeout = 5 * time.Second

type waylandSession struct {
	app           *Application
	native        *waylandNative
	display       unsafe.Pointer
	registry      unsafe.Pointer
	compositor    unsafe.Pointer
	shm           unsafe.Pointer
	shell         unsafe.Pointer
	manager       unsafe.Pointer
	surface       unsafe.Pointer
	layer         unsafe.Pointer
	cursor        unsafe.Pointer
	cursorBuffer  *waylandBuffer
	xkbContext    unsafe.Pointer
	objects       map[unsafe.Pointer]func(uint32, []waylandArgument)
	globals       map[uint32]unsafe.Pointer
	seats         map[unsafe.Pointer]*waylandSeat
	outputs       map[unsafe.Pointer]int
	entered       map[unsafe.Pointer]bool
	buffers       []*waylandBuffer
	logicalWidth  int
	logicalHeight int
	configured    bool
	dirty         bool
	active        bool
	done          bool
	err           error
	deadline      time.Time
}

var waylandSessions sync.Map

var waylandDispatcher = purego.NewCallback(dispatchWaylandEvent)

func (app *Application) runWayland() error {
	// libwayland dispatches callbacks synchronously on this thread. Keep all
	// protocol objects and keyboard state confined to that event loop.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	native, err := loadWaylandNative()
	if err != nil {
		return err
	}

	defer native.close()

	session := &waylandSession{
		app:      app,
		native:   native,
		objects:  make(map[unsafe.Pointer]func(uint32, []waylandArgument), 32),
		globals:  make(map[uint32]unsafe.Pointer, 16),
		seats:    make(map[unsafe.Pointer]*waylandSeat, 2),
		outputs:  make(map[unsafe.Pointer]int, 4),
		entered:  make(map[unsafe.Pointer]bool, 4),
		buffers:  make([]*waylandBuffer, 0, 2),
		deadline: time.Now().Add(waylandStartupTimeout),
	}

	defer session.close()

	var displayName *byte

	name := os.Getenv("WAYLAND_DISPLAY")
	if name != "" {
		displayName = native.text(name)
	}

	session.display = native.connect(displayName)
	if session.display == nil {
		return fmt.Errorf("connect to Wayland display; XWayland cannot provide a desktop-wide keyboard lock")
	}

	waylandSessions.Store(session.display, session)

	app.lockPending = true
	app.uiScale = 1

	session.xkbContext = native.xkbContextNew(0)
	if session.xkbContext == nil {
		return fmt.Errorf("create Wayland keyboard layout context")
	}

	session.registry = session.create(session.display, 1, "wl_registry", 1, 0)
	session.listen(session.registry, session.registryEvent)

	err = session.roundTrip()
	if err != nil {
		return err
	}

	err = session.checkProtocols()
	if err != nil {
		return err
	}

	err = session.roundTrip()
	if err != nil {
		return err
	}

	if !session.hasKeyboard() {
		return fmt.Errorf("the Wayland desktop has no available keyboard")
	}

	session.surface = session.create(session.compositor, 0, "wl_surface", 4, 0)
	session.listen(session.surface, session.surfaceEvent)

	err = session.createCursor()
	if err != nil {
		return err
	}

	namespace := native.text("catlock")

	session.layer = session.create(session.shell, 0, "zwlr_layer_surface_v1", 1, 0, waylandPointer(session.surface), 0, 3, waylandPointer(unsafe.Pointer(namespace)))
	session.listen(session.layer, session.layerEvent)
	session.request(session.layer, 4, 1)
	session.request(session.layer, 2, waylandArgument(uint32(0xffffffff)))
	session.setLayout()

	for _, seat := range session.seats {
		seat.inhibit()
	}

	session.request(session.surface, 6)

	return session.runEvents()
}

func (session *waylandSession) close() {
	for _, seat := range session.seats {
		seat.closeKeymap()
	}

	if session.xkbContext != nil {
		session.native.xkbContextUnref(session.xkbContext)
	}

	if session.display != nil {
		// Closing the connection is the release mechanism, including on crashes.
		// Local proxy destruction also frees libwayland's client-side allocations.
		for proxy := range session.objects {
			session.native.destroy(proxy)
		}

		session.native.disconnect(session.display)
		waylandSessions.Delete(session.display)
	}

	for _, buffer := range session.buffers {
		_ = unix.Munmap(buffer.pixels)
	}

	if session.cursorBuffer != nil {
		_ = unix.Munmap(session.cursorBuffer.pixels)
	}

	if session.app.renderer != nil {
		session.app.renderer.close()
	}
}

func (session *waylandSession) fail(err error) {
	if session.err == nil {
		session.err = err
	}
}

func (session *waylandSession) create(parent unsafe.Pointer, opcode uint32, name string, version uint32, arguments ...waylandArgument) unsafe.Pointer {
	if parent == nil || session.err != nil {
		return nil
	}

	proxy := session.native.marshal(parent, opcode, session.native.interfaces[name], version, 0, unsafe.SliceData(arguments))
	if proxy == nil {
		session.fail(fmt.Errorf("create Wayland object %s", name))

		return nil
	}

	session.objects[proxy] = nil

	return proxy
}

func (session *waylandSession) request(proxy unsafe.Pointer, opcode uint32, arguments ...waylandArgument) {
	if proxy != nil && session.err == nil {
		session.native.marshal(proxy, opcode, nil, session.native.version(proxy), 0, unsafe.SliceData(arguments))
	}
}

func (session *waylandSession) destroy(proxy unsafe.Pointer, opcode uint32) {
	if proxy == nil {
		return
	}

	session.native.marshal(proxy, opcode, nil, session.native.version(proxy), 1, nil)
	delete(session.objects, proxy)
}

func (session *waylandSession) listen(proxy unsafe.Pointer, handler func(uint32, []waylandArgument)) {
	if proxy == nil || session.err != nil {
		return
	}

	session.objects[proxy] = handler

	result := session.native.addDispatcher(proxy, waylandDispatcher, session.display, nil)
	if result != 0 {
		session.fail(fmt.Errorf("register Wayland event dispatcher"))
	}
}

func (session *waylandSession) bind(name uint32, protocol string, version uint32) unsafe.Pointer {
	interfaceName := session.native.interfaces[protocol].name

	proxy := session.create(session.registry, 0, protocol, version, waylandArgument(name), waylandPointer(unsafe.Pointer(interfaceName)), waylandArgument(version), 0)
	session.globals[name] = proxy

	return proxy
}

func (session *waylandSession) registryEvent(opcode uint32, arguments []waylandArgument) {
	name := uint32(arguments[0])

	if opcode == 1 {
		proxy := session.globals[name]

		seat := session.seats[proxy]
		if seat != nil && seat.keyboard != nil {
			session.fail(fmt.Errorf("the Wayland keyboard seat was removed; keyboard released"))
		}

		if proxy == session.shell || proxy == session.manager || proxy == session.compositor || proxy == session.shm {
			session.fail(fmt.Errorf("the Wayland desktop withdrew a required locking capability"))
		}

		delete(session.outputs, proxy)
		delete(session.entered, proxy)
		delete(session.globals, name)

		session.dirty = true

		return
	}

	protocol := waylandText((*byte)(arguments[1].pointer()))
	version := uint32(arguments[2])

	switch protocol {
	case "wl_compositor":
		if version >= 4 {
			session.compositor = session.bind(name, protocol, 4)
		}
	case "wl_shm":
		session.shm = session.bind(name, protocol, 1)
	case "zwlr_layer_shell_v1":
		session.shell = session.bind(name, protocol, 1)
	case "zwp_keyboard_shortcuts_inhibit_manager_v1":
		session.manager = session.bind(name, protocol, 1)
	case "wl_seat":
		proxy := session.bind(name, protocol, min(version, 5))
		seat := &waylandSeat{session: session, proxy: proxy}
		session.seats[proxy] = seat
		session.listen(proxy, seat.event)
	case "wl_output":
		proxy := session.bind(name, protocol, min(version, 2))
		session.outputs[proxy] = 1

		session.listen(proxy, func(event uint32, values []waylandArgument) {
			if event == 3 {
				session.outputs[proxy] = min(max(int(int32(values[0])), 1), maxUIScale)
				session.dirty = true
			}
		})
	}
}

func (session *waylandSession) checkProtocols() error {
	if session.compositor == nil || session.shm == nil {
		return fmt.Errorf("unsupported Wayland desktop: wl_compositor version 4 and shared-memory buffers are required")
	}

	if session.shell == nil || session.manager == nil {
		return fmt.Errorf("unsupported Wayland desktop: CatLock needs layer-shell and keyboard-shortcut inhibition for a keyboard lock; use a compatible Wayland desktop or a native X11 session")
	}

	return session.err
}

func (session *waylandSession) roundTrip() error {
	done := false
	callback := session.create(session.display, 0, "wl_callback", 1, 0)

	session.listen(callback, func(_ uint32, _ []waylandArgument) {
		done = true
	})

	for !done && session.err == nil {
		if time.Now().After(session.deadline) {
			session.fail(fmt.Errorf("the Wayland desktop did not respond during keyboard lock setup"))

			break
		}

		session.pump()
	}

	if callback != nil {
		session.native.destroy(callback)
		delete(session.objects, callback)
	}

	return session.err
}

func (session *waylandSession) pump() {
	for session.native.prepareRead(session.display) != 0 {
		if session.native.dispatch(session.display) < 0 {
			session.fail(fmt.Errorf("the Wayland connection failed"))

			return
		}

		if session.done || session.err != nil {
			return
		}
	}

	events := int16(unix.POLLIN)

	if session.native.flush(session.display) < 0 {
		if session.native.getError(session.display) != 0 {
			session.native.cancelRead(session.display)
			session.fail(fmt.Errorf("flush Wayland requests: connection lost"))

			return
		}

		events |= unix.POLLOUT
	}

	descriptors := [1]unix.PollFd{{Fd: session.native.getFD(session.display), Events: events}}
	count, err := unix.Poll(descriptors[:], 100)

	if err != nil || count == 0 || descriptors[0].Revents&unix.POLLIN == 0 {
		session.native.cancelRead(session.display)

		if err != nil && !errors.Is(err, unix.EINTR) {
			session.fail(fmt.Errorf("wait for Wayland input: %w", err))
		}

		if descriptors[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			session.fail(fmt.Errorf("the Wayland display disconnected"))
		}

		return
	}

	if session.native.readEvents(session.display) < 0 || session.native.dispatch(session.display) < 0 {
		session.fail(fmt.Errorf("read Wayland input: connection lost"))
	}
}

func (session *waylandSession) runEvents() error {
	for !session.done && session.err == nil {
		session.updateLockState()

		if !session.active && time.Now().After(session.deadline) {
			session.fail(fmt.Errorf("the Wayland desktop did not grant exclusive keyboard focus and shortcut inhibition; keyboard released"))
		}

		if session.err != nil {
			break
		}

		if session.configured && session.dirty {
			err := session.paint()
			if err != nil {
				session.fail(err)

				break
			}
		}

		session.pump()
	}

	return session.err
}

func (session *waylandSession) surfaceEvent(opcode uint32, arguments []waylandArgument) {
	output := arguments[0].pointer()

	switch opcode {
	case 0:
		session.entered[output] = true
	case 1:
		delete(session.entered, output)
	}

	session.dirty = true
}

func (session *waylandSession) layerEvent(opcode uint32, arguments []waylandArgument) {
	if opcode == 1 {
		session.fail(fmt.Errorf("the Wayland desktop closed the keyboard lock; keyboard released"))

		return
	}

	session.request(session.layer, 6, arguments[0])
	width := int(uint32(arguments[1]))
	height := int(uint32(arguments[2]))

	if width != 0 {
		session.logicalWidth = width
	}

	if height != 0 {
		session.logicalHeight = height
	}

	session.configured = true
	session.dirty = true
}

func (session *waylandSession) setLayout() {
	width := preferredWidth
	height := preferredHeight

	var (
		anchor waylandArgument
		margin waylandArgument
	)

	if session.app.compact {
		width = compactWidth
		height = compactHeight
		anchor = 1 | 4
		margin = compactMargin
	}

	session.logicalWidth = width
	session.logicalHeight = height
	session.request(session.layer, 0, waylandArgument(width), waylandArgument(height))
	session.request(session.layer, 1, anchor)
	session.request(session.layer, 3, margin, margin, margin, margin)
	session.app.button = Rect{}
	session.app.logButton = Rect{}
	session.app.toggleButton = Rect{}
	session.configured = false
	session.dirty = true
}

func (session *waylandSession) hasKeyboard() bool {
	for _, seat := range session.seats {
		if seat.keyboard != nil {
			return true
		}
	}

	return false
}

func (session *waylandSession) updateLockState() {
	ready := session.hasKeyboard()

	for _, seat := range session.seats {
		if seat.keyboard != nil && (!seat.focused || !seat.inhibited || seat.state == nil) {
			ready = false
		}
	}

	if session.active && !ready {
		session.fail(fmt.Errorf("the Wayland keyboard focus or shortcut inhibition was lost; keyboard released"))
	}

	if ready && !session.active {
		session.active = true
		session.app.lockPending = false
		session.dirty = true
	}
}

func dispatchWaylandEvent(implementation unsafe.Pointer, proxy unsafe.Pointer, opcode uint32, message *waylandMessage, arguments *waylandArgument) uintptr {
	value, found := waylandSessions.Load(implementation)
	if !found {
		return 0
	}

	session := value.(*waylandSession)

	handler := session.objects[proxy]
	if handler == nil {
		return 0
	}

	count := 0

	for _, character := range waylandText(message.signature) {
		if character != '?' && (character < '0' || character > '9') {
			count++
		}
	}

	handler(opcode, unsafe.Slice(arguments, count))

	return 0
}
