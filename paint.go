//go:build windows

package main

import (
	"fmt"
	"unsafe"
)

func (app *Application) paint(hwnd uintptr) {
	var paint paintStruct

	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if hdc == 0 {
		return
	}

	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))

	var client Rect

	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))

	background := rgb(32, 32, 32)
	surface := rgb(44, 44, 44)
	border := rgb(63, 63, 63)
	separator := rgb(55, 55, 55)
	foreground := rgb(243, 243, 243)
	secondary := rgb(174, 174, 174)
	quiet := rgb(139, 139, 139)
	accent := rgb(204, 36, 29) // Gruvbox red.
	button := rgb(15, 108, 189)
	buttonText := rgb(255, 255, 255)

	width := client.right - client.left
	height := client.bottom - client.top

	// A one-pixel border provides definition without a traditional title bar.
	fillRect(hdc, client, border)

	panel := Rect{
		left:   1,
		top:    1,
		right:  width - 1,
		bottom: height - 1,
	}

	fillRect(hdc, panel, background)

	accentBar := Rect{
		left:   1,
		top:    1,
		right:  6,
		bottom: height - 1,
	}

	fillRect(hdc, accentBar, accent)

	title := Rect{
		left:   28,
		top:    17,
		right:  width / 2,
		bottom: 52,
	}

	drawText(
		hdc,
		"Keyboard locked",
		title,
		23,
		600,
		foreground,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	state := Rect{
		left:   width / 2,
		top:    17,
		right:  width - 24,
		bottom: 52,
	}

	drawText(
		hdc,
		"RECORDING LOCALLY",
		state,
		12,
		600,
		accent,
		dtRight|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	headerSeparator := Rect{
		left:   28,
		top:    62,
		right:  width - 24,
		bottom: 63,
	}

	fillRect(hdc, headerSeparator, separator)

	description := Rect{
		left:   28,
		top:    73,
		right:  width - 28,
		bottom: 105,
	}

	drawText(
		hdc,
		"Input is captured here and will not reach your other applications.",
		description,
		15,
		400,
		secondary,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	status := Rect{
		left:   28,
		top:    116,
		right:  width - 24,
		bottom: 158,
	}

	fillRect(hdc, status, surface)

	statusLabel := status
	statusLabel.left += 14
	statusLabel.right = width / 2

	drawText(
		hdc,
		"Captured input",
		statusLabel,
		13,
		400,
		secondary,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	count := status
	count.left = width / 2
	count.right -= 14

	drawText(
		hdc,
		fmt.Sprintf("%d key presses", app.keyCount),
		count,
		14,
		600,
		foreground,
		dtRight|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	pathLabel := Rect{
		left:   28,
		top:    170,
		right:  width - 28,
		bottom: 190,
	}

	drawText(
		hdc,
		"Capture file",
		pathLabel,
		12,
		600,
		quiet,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	pathArea := Rect{
		left:   28,
		top:    191,
		right:  width - 28,
		bottom: 218,
	}

	drawText(
		hdc,
		app.capturePath,
		pathArea,
		13,
		400,
		secondary,
		dtVCenter|dtSingleLine|dtPathEllipsis|dtNoPrefix,
	)

	buttonWidth := min(int32(184), width/2)

	app.button = Rect{
		left:   width - buttonWidth - 20,
		top:    height - 48,
		right:  width - 20,
		bottom: height - 16,
	}

	fillRect(hdc, app.button, button)

	drawText(
		hdc,
		"Unlock",
		app.button,
		13,
		600,
		buttonText,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	emergency := Rect{
		left:   28,
		top:    height - 48,
		right:  app.button.left - 16,
		bottom: height - 16,
	}

	drawText(
		hdc,
		"Ctrl + Alt + Shift + F12",
		emergency,
		12,
		400,
		quiet,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)
}

func fillRect(hdc uintptr, area Rect, color uint32) {
	brush, _, _ := procCreateSolidBrush.Call(uintptr(color))
	if brush == 0 {
		return
	}

	defer procDeleteObject.Call(brush)

	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&area)), brush)
}

func drawText(hdc uintptr, text string, area Rect, size int32, weight int32, color uint32, format uintptr) {
	face := utf16Ptr("Segoe UI Variable Text")

	font, _, _ := procCreateFontW.Call(
		uintptr(-size),
		0,
		0,
		0,
		uintptr(weight),
		0,
		0,
		0,
		1,
		0,
		0,
		cleartypeQuality,
		0,
		uintptr(unsafe.Pointer(face)),
	)

	if font == 0 {
		return
	}

	defer procDeleteObject.Call(font)

	oldFont, _, _ := procSelectObject.Call(hdc, font)
	defer procSelectObject.Call(hdc, oldFont)

	procSetBkMode.Call(hdc, transparent)
	procSetTextColor.Call(hdc, uintptr(color))

	textPtr := utf16Ptr(text)

	procDrawTextW.Call(
		hdc,
		uintptr(unsafe.Pointer(textPtr)),
		^uintptr(0),
		uintptr(unsafe.Pointer(&area)),
		format,
	)
}

func rgb(red, green, blue byte) uint32 {
	return uint32(red) | uint32(green)<<8 | uint32(blue)<<16
}
