//go:build linux

package linux

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// These layouts mirror wl_interface, wl_message and the pointer-sized
// wl_argument union. Native pointers are borrowed only while their owner lives.
type waylandInterface struct {
	name        *byte
	version     int32
	methodCount int32
	methods     *waylandMessage
	eventCount  int32
	events      *waylandMessage
}

type waylandMessage struct {
	name      *byte
	signature *byte
	types     **waylandInterface
}

type waylandArgument uintptr

type nativeSymbol struct {
	name   string
	target any
}

type waylandNative struct {
	library         uintptr
	xkbLibrary      uintptr
	pins            runtime.Pinner
	interfaces      map[string]*waylandInterface
	connect         func(*byte) unsafe.Pointer
	disconnect      func(unsafe.Pointer)
	getFD           func(unsafe.Pointer) int32
	prepareRead     func(unsafe.Pointer) int32
	cancelRead      func(unsafe.Pointer)
	readEvents      func(unsafe.Pointer) int32
	dispatch        func(unsafe.Pointer) int32
	flush           func(unsafe.Pointer) int32
	getError        func(unsafe.Pointer) int32
	marshal         func(unsafe.Pointer, uint32, *waylandInterface, uint32, uint32, *waylandArgument) unsafe.Pointer
	version         func(unsafe.Pointer) uint32
	destroy         func(unsafe.Pointer)
	addDispatcher   func(unsafe.Pointer, uintptr, unsafe.Pointer, unsafe.Pointer) int32
	xkbContextNew   func(uint32) unsafe.Pointer
	xkbContextUnref func(unsafe.Pointer)
	xkbKeymapNew    func(unsafe.Pointer, *byte, uint32, uint32) unsafe.Pointer
	xkbKeymapUnref  func(unsafe.Pointer)
	xkbStateNew     func(unsafe.Pointer) unsafe.Pointer
	xkbStateUnref   func(unsafe.Pointer)
	xkbUpdateMask   func(unsafe.Pointer, uint32, uint32, uint32, uint32, uint32, uint32) uint32
	xkbKeySym       func(unsafe.Pointer, uint32) uint32
	xkbKeyLayout    func(unsafe.Pointer, uint32) uint32
	xkbKeySymbols   func(unsafe.Pointer, uint32, uint32, uint32, **uint32) int32
	xkbKeyUTF8      func(unsafe.Pointer, uint32, *byte, uintptr) int32
	xkbModActive    func(unsafe.Pointer, string, uint32) int32
}

func (argument waylandArgument) pointer() unsafe.Pointer {
	// Reinterpret the union's object/string member, rather than manufacturing
	// a Go pointer by arithmetic on an integer address.
	return *(*unsafe.Pointer)(unsafe.Pointer(&argument))
}

func (native *waylandNative) close() {
	native.pins.Unpin()

	if native.xkbLibrary != 0 {
		_ = purego.Dlclose(native.xkbLibrary)
	}

	if native.library != 0 {
		_ = purego.Dlclose(native.library)
	}
}

func (native *waylandNative) text(value string) *byte {
	text, _ := unix.BytePtrFromString(value)
	native.pins.Pin(text)

	return text
}

func (native *waylandNative) protocol(name string, methods, events []waylandMessage) *waylandInterface {
	protocol := &waylandInterface{
		name:        native.text(name),
		version:     1,
		methodCount: int32(len(methods)),
		methods:     unsafe.SliceData(methods),
		eventCount:  int32(len(events)),
		events:      unsafe.SliceData(events),
	}

	native.pins.Pin(protocol)

	if len(methods) != 0 {
		native.pins.Pin(&methods[0])
	}

	if len(events) != 0 {
		native.pins.Pin(&events[0])
	}

	native.interfaces[name] = protocol

	return protocol
}

func (native *waylandNative) message(name, signature string, types ...*waylandInterface) waylandMessage {
	if len(types) != 0 {
		native.pins.Pin(&types[0])
	}

	return waylandMessage{
		name:      native.text(name),
		signature: native.text(signature),
		types:     unsafe.SliceData(types),
	}
}

func (native *waylandNative) defineProtocols() {
	layer := native.protocol("zwlr_layer_surface_v1", nil, nil)

	layerMethods := []waylandMessage{
		native.message("set_size", "uu"),
		native.message("set_anchor", "u"),
		native.message("set_exclusive_zone", "i"),
		native.message("set_margin", "iiii"),
		native.message("set_keyboard_interactivity", "u"),
		native.message("get_popup", "o"),
		native.message("ack_configure", "u"),
		native.message("destroy", ""),
	}

	layerEvents := []waylandMessage{
		native.message("configure", "uuu"),
		native.message("closed", ""),
	}

	native.pins.Pin(&layerMethods[0])
	native.pins.Pin(&layerEvents[0])

	layer.methodCount = int32(len(layerMethods))
	layer.methods = &layerMethods[0]
	layer.eventCount = int32(len(layerEvents))
	layer.events = &layerEvents[0]

	shellMethods := []waylandMessage{
		native.message("get_layer_surface", "no?ous", layer, native.interfaces["wl_surface"], native.interfaces["wl_output"], nil, nil),
	}

	native.protocol("zwlr_layer_shell_v1", shellMethods, nil)

	inhibitorMethods := []waylandMessage{native.message("destroy", "")}
	inhibitorEvents := []waylandMessage{native.message("active", ""), native.message("inactive", "")}

	inhibitor := native.protocol("zwp_keyboard_shortcuts_inhibitor_v1", inhibitorMethods, inhibitorEvents)

	managerMethods := []waylandMessage{
		native.message("destroy", ""),
		native.message("inhibit_shortcuts", "noo", inhibitor, native.interfaces["wl_surface"], native.interfaces["wl_seat"]),
	}

	native.protocol("zwp_keyboard_shortcuts_inhibit_manager_v1", managerMethods, nil)
}

func loadWaylandNative() (*waylandNative, error) {
	native := &waylandNative{interfaces: make(map[string]*waylandInterface, 16)}

	var err error

	native.library, err = purego.Dlopen("libwayland-client.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("load Wayland client library: %w", err)
	}

	native.xkbLibrary, err = purego.Dlopen("libxkbcommon.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		native.close()

		return nil, fmt.Errorf("load keyboard layout library: %w", err)
	}

	waylandSymbols := []nativeSymbol{
		{"wl_display_connect", &native.connect},
		{"wl_display_disconnect", &native.disconnect},
		{"wl_display_get_fd", &native.getFD},
		{"wl_display_prepare_read", &native.prepareRead},
		{"wl_display_cancel_read", &native.cancelRead},
		{"wl_display_read_events", &native.readEvents},
		{"wl_display_dispatch_pending", &native.dispatch},
		{"wl_display_flush", &native.flush},
		{"wl_display_get_error", &native.getError},
		{"wl_proxy_marshal_array_flags", &native.marshal},
		{"wl_proxy_get_version", &native.version},
		{"wl_proxy_destroy", &native.destroy},
		{"wl_proxy_add_dispatcher", &native.addDispatcher},
	}

	xkbSymbols := []nativeSymbol{
		{"xkb_context_new", &native.xkbContextNew},
		{"xkb_context_unref", &native.xkbContextUnref},
		{"xkb_keymap_new_from_string", &native.xkbKeymapNew},
		{"xkb_keymap_unref", &native.xkbKeymapUnref},
		{"xkb_state_new", &native.xkbStateNew},
		{"xkb_state_unref", &native.xkbStateUnref},
		{"xkb_state_update_mask", &native.xkbUpdateMask},
		{"xkb_state_key_get_one_sym", &native.xkbKeySym},
		{"xkb_state_key_get_layout", &native.xkbKeyLayout},
		{"xkb_keymap_key_get_syms_by_level", &native.xkbKeySymbols},
		{"xkb_state_key_get_utf8", &native.xkbKeyUTF8},
		{"xkb_state_mod_name_is_active", &native.xkbModActive},
	}

	err = registerNativeSymbols(native.library, waylandSymbols)
	if err == nil {
		err = registerNativeSymbols(native.xkbLibrary, xkbSymbols)
	}

	if err != nil {
		native.close()

		return nil, err
	}

	// Use a typed dlsym binding for data symbols: these are C-owned interface
	// descriptors, not integer addresses converted into Go-owned pointers.
	var lookup func(uintptr, string) *waylandInterface

	lookupSymbols := []nativeSymbol{{"dlsym", &lookup}}

	err = registerNativeSymbols(native.library, lookupSymbols)
	if err != nil {
		native.close()

		return nil, err
	}

	interfaceNames := []string{"wl_registry", "wl_callback", "wl_compositor", "wl_surface", "wl_shm", "wl_shm_pool", "wl_buffer", "wl_seat", "wl_keyboard", "wl_pointer", "wl_output"}

	for _, name := range interfaceNames {
		protocol := lookup(native.library, name+"_interface")
		if protocol == nil {
			native.close()

			return nil, fmt.Errorf("the Wayland library is missing %s", name)
		}

		native.interfaces[name] = protocol
	}

	native.defineProtocols()

	return native, nil
}

func registerNativeSymbols(library uintptr, symbols []nativeSymbol) error {
	for _, symbol := range symbols {
		address, err := purego.Dlsym(library, symbol.name)
		if err != nil {
			return fmt.Errorf("load native function %s: %w", symbol.name, err)
		}

		purego.RegisterFunc(symbol.target, address)
	}

	return nil
}

func waylandPointer(pointer unsafe.Pointer) waylandArgument {
	return waylandArgument(uintptr(pointer))
}

func waylandText(pointer *byte) string {
	if pointer == nil {
		return ""
	}

	length := 0

	for *(*byte)(unsafe.Add(unsafe.Pointer(pointer), length)) != 0 {
		length++
	}

	// The borrowed view never outlives the native event or pinned descriptor.
	return unsafe.String(pointer, length)
}
