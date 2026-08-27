//go:build windows

package main

import "unsafe"

type Application struct {
	hwnd uintptr
	hook uintptr

	capture     *CaptureLog
	capturePath string
	keyCount    uint64

	button Rect

	keysDown [256]bool
	shift    bool
	ctrl     bool
	alt      bool
	win      bool
	caps     bool

	windowX      int32
	windowY      int32
	windowWidth  int32
	windowHeight int32
}

func (app *Application) runUI() error {
	app.refreshMetrics()

	instance, _, lastErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return win32Error("GetModuleHandleW", lastErr)
	}

	className := utf16Ptr("CatLockWindowClass")
	title := utf16Ptr("CatLock")

	cursor, _, lastErr := procLoadCursorW.Call(0, 32512)
	if cursor == 0 {
		return win32Error("LoadCursorW", lastErr)
	}

	windowClass := windowClassEx{
		size:       uint32(unsafe.Sizeof(windowClassEx{})),
		style:      csDropShadow,
		windowProc: windowCallback,
		instance:   instance,
		cursor:     cursor,
		className:  className,
	}

	atom, _, lastErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&windowClass)))
	if atom == 0 {
		return win32Error("RegisterClassExW", lastErr)
	}

	hwnd, _, lastErr := procCreateWindowExW.Call(
		wsExTopmost|wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		uintptr(app.windowX),
		uintptr(app.windowY),
		uintptr(app.windowWidth),
		uintptr(app.windowHeight),
		0,
		0,
		instance,
		0,
	)

	if hwnd == 0 {
		return win32Error("CreateWindowExW", lastErr)
	}

	app.hwnd = hwnd

	module, _, lastErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		procDestroyWindow.Call(hwnd)

		return win32Error("GetModuleHandleW", lastErr)
	}

	hook, _, lastErr := procSetWindowsHookExW.Call(whKeyboardLL, keyboardCallback, module, 0)
	if hook == 0 {
		procDestroyWindow.Call(hwnd)

		return win32Error("SetWindowsHookExW", lastErr)
	}

	app.hook = hook

	defer procUnhookWindowsHookEx.Call(hook)

	capsState, _, _ := procGetKeyState.Call(vkCapital)
	app.caps = int16(capsState&0xffff)&1 != 0

	timer, _, lastErr := procSetTimer.Call(hwnd, 1, 1000, 0)
	if timer == 0 {
		procDestroyWindow.Call(hwnd)

		return win32Error("SetTimer", lastErr)
	}

	defer procKillTimer.Call(hwnd, timer)

	procSetWindowPos.Call(
		hwnd,
		^uintptr(0), // HWND_TOPMOST
		uintptr(app.windowX),
		uintptr(app.windowY),
		uintptr(app.windowWidth),
		uintptr(app.windowHeight),
		0,
	)

	procShowWindow.Call(hwnd, swShow)

	var message Message

	for {
		result, _, lastErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)

		switch int32(result) {
		case -1:
			return win32Error("GetMessageW", lastErr)
		case 0:
			return nil
		}

		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func (app *Application) refreshMetrics() {
	screenWidth := max(systemMetric(smCXScreen), int32(1))
	screenHeight := max(systemMetric(smCYScreen), int32(1))

	margin := preferredMargin
	if screenWidth <= 2*margin || screenHeight <= 2*margin {
		margin = 0
	}

	app.windowWidth = min(preferredWidth, screenWidth-2*margin)
	app.windowHeight = min(preferredHeight, screenHeight-2*margin)

	app.windowX = (screenWidth - app.windowWidth) / 2
	app.windowY = (screenHeight - app.windowHeight) / 2
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	app := activeApp
	if app == nil {
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)

		return result
	}

	switch message {
	case wmPaint:
		app.paint(hwnd)

		return 0
	case wmEraseBkgnd:
		return 1
	case wmCapture:
		procInvalidateRect.Call(hwnd, 0, 0)

		return 0
	case wmLButtonUp:
		var cursor Point

		result, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
		if result != 0 {
			procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&cursor)))

			if app.button.contains(cursor) {
				procPostMessageW.Call(hwnd, wmUnlock, 0, 0)
			}
		}

		return 0
	case wmUnlock:
		procDestroyWindow.Call(hwnd)

		return 0
	case wmClose:
		// The frameless window has no close button, ignoring WM_CLOSE prevents
		return 0
	case wmDisplayChange:
		app.refreshMetrics()

		procSetWindowPos.Call(
			hwnd,
			^uintptr(0),
			uintptr(app.windowX),
			uintptr(app.windowY),
			uintptr(app.windowWidth),
			uintptr(app.windowHeight),
			0,
		)

		procInvalidateRect.Call(hwnd, 0, 0)

		return 0
	case wmTimer:
		// Reassert the topmost position in case another topmost window appeared.
		procSetWindowPos.Call(
			hwnd,
			^uintptr(0),
			0,
			0,
			0,
			0,
			swpNoMove|swpNoSize|swpNoActivate,
		)

		return 0
	case wmDestroy:
		app.hwnd = 0

		procPostQuitMessage.Call(0)

		return 0
	}

	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}
