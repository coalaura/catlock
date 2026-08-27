//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	whKeyboardLL = 13
	csDropShadow = 0x00020000

	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmDisplayChange = 0x007E
	wmTimer         = 0x0113
	wmKeyDown       = 0x0100
	wmKeyUp         = 0x0101
	wmSysKeyDown    = 0x0104
	wmSysKeyUp      = 0x0105
	wmLButtonUp     = 0x0202
	wmApp           = 0x8000

	wmCapture = wmApp + 1
	wmUnlock  = wmApp + 2

	wsPopup        = 0x80000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080

	swShow = 5

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010

	smCXScreen  = 0
	smCYScreen  = 1
	vkBack      = 0x08
	vkTab       = 0x09
	vkReturn    = 0x0D
	vkShift     = 0x10
	vkControl   = 0x11
	vkMenu      = 0x12
	vkPause     = 0x13
	vkCapital   = 0x14
	vkEscape    = 0x1B
	vkSpace     = 0x20
	vkPrior     = 0x21
	vkNext      = 0x22
	vkEnd       = 0x23
	vkHome      = 0x24
	vkLeft      = 0x25
	vkUp        = 0x26
	vkRight     = 0x27
	vkDown      = 0x28
	vkSnapshot  = 0x2C
	vkInsert    = 0x2D
	vkDelete    = 0x2E
	vkLWin      = 0x5B
	vkRWin      = 0x5C
	vkNumpad0   = 0x60
	vkMultiply  = 0x6A
	vkAdd       = 0x6B
	vkSubtract  = 0x6D
	vkDecimal   = 0x6E
	vkDivide    = 0x6F
	vkF1        = 0x70
	vkF12       = 0x7B
	vkF24       = 0x87
	vkNumLock   = 0x90
	vkScroll    = 0x91
	vkLShift    = 0xA0
	vkRShift    = 0xA1
	vkLControl  = 0xA2
	vkRControl  = 0xA3
	vkLMenu     = 0xA4
	vkRMenu     = 0xA5
	vkOEM1      = 0xBA
	vkOEMPlus   = 0xBB
	vkOEMComma  = 0xBC
	vkOEMMinus  = 0xBD
	vkOEMPeriod = 0xBE
	vkOEM2      = 0xBF
	vkOEM3      = 0xC0
	vkOEM4      = 0xDB
	vkOEM5      = 0xDC
	vkOEM6      = 0xDD
	vkOEM7      = 0xDE
	vkOEM102    = 0xE2

	dtCenter       = 0x0001
	dtRight        = 0x0002
	dtVCenter      = 0x0004
	dtSingleLine   = 0x0020
	dtNoPrefix     = 0x0800
	dtPathEllipsis = 0x4000

	transparent      = 1
	cleartypeQuality = 5

	preferredWidth  int32 = 700
	preferredHeight int32 = 350
	preferredMargin int32 = 32
)

type Point struct {
	x int32
	y int32
}

type Rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type Message struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   Point
	private uint32
}

type paintStruct struct {
	hdc       uintptr
	erase     int32
	paint     Rect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

type windowClassEx struct {
	size        uint32
	style       uint32
	windowProc  uintptr
	classExtra  int32
	windowExtra int32
	instance    uintptr
	icon        uintptr
	cursor      uintptr
	background  uintptr
	menuName    *uint16
	className   *uint16
	smallIcon   uintptr
}

type keyboardEvent struct {
	virtualKey uint32
	scanCode   uint32
	flags      uint32
	time       uint32
	extraInfo  uintptr
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procBeginPaint          = user32.NewProc("BeginPaint")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procDrawTextW           = user32.NewProc("DrawTextW")
	procEndPaint            = user32.NewProc("EndPaint")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetKeyState         = user32.NewProc("GetKeyState")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procInvalidateRect      = user32.NewProc("InvalidateRect")
	procKillTimer           = user32.NewProc("KillTimer")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procScreenToClient      = user32.NewProc("ScreenToClient")
	procSetTimer            = user32.NewProc("SetTimer")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")

	procCreateFontW      = gdi32.NewProc("CreateFontW")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procSelectObject     = gdi32.NewProc("SelectObject")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procSetTextColor     = gdi32.NewProc("SetTextColor")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")

	procFillRect = user32.NewProc("FillRect")

	windowCallback   = syscall.NewCallback(windowProc)
	keyboardCallback = syscall.NewCallback(keyboardProc)

	activeApp *Application
)

func (area Rect) contains(point Point) bool {
	return point.x >= area.left && point.x < area.right && point.y >= area.top && point.y < area.bottom
}

func systemMetric(index int32) int32 {
	result, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int32(result)
}

func utf16Ptr(text string) *uint16 {
	ptr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		panic(err)
	}

	return ptr
}

func win32Error(operation string, lastErr error) error {
	if lastErr == nil || lastErr == syscall.Errno(0) {
		return fmt.Errorf("%s failed", operation)
	}

	return fmt.Errorf("%s: %w", operation, lastErr)
}

func showError(err error) {
	title := utf16Ptr("CatLock error")
	message := utf16Ptr(err.Error())

	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(message)),
		uintptr(unsafe.Pointer(title)),
		0x10, // MB_ICONERROR
	)
}
