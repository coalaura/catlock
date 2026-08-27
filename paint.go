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

	background := rgb(27, 32, 39)
	surface := rgb(36, 43, 52)
	surfaceRaised := rgb(42, 50, 60)
	border := rgb(73, 83, 96)
	separator := rgb(53, 62, 73)
	foreground := rgb(235, 239, 243)
	secondary := rgb(169, 179, 190)
	quiet := rgb(126, 138, 151)
	accent := rgb(105, 172, 158)
	accentSurface := rgb(38, 69, 66)
	button := rgb(66, 111, 123)
	buttonText := rgb(247, 250, 251)

	width := client.right - client.left
	height := client.bottom - client.top

	// The layered edge and class drop shadow distinguish the frameless window.
	fillRect(hdc, client, border)

	panel := Rect{
		left:   2,
		top:    2,
		right:  width - 2,
		bottom: height - 2,
	}

	fillRect(hdc, panel, background)

	topAccent := Rect{
		left:   2,
		top:    2,
		right:  width - 2,
		bottom: 5,
	}

	fillRect(hdc, topAccent, accent)

	mark := Rect{
		left:   28,
		top:    24,
		right:  68,
		bottom: 64,
	}

	fillRect(hdc, mark, surfaceRaised)

	drawText(
		hdc,
		"C",
		mark,
		18,
		600,
		accent,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	title := Rect{
		left:   84,
		top:    20,
		right:  width - 180,
		bottom: 47,
	}

	drawText(
		hdc,
		"Keyboard input paused",
		title,
		21,
		600,
		foreground,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	subtitle := Rect{
		left:   84,
		top:    47,
		right:  width - 180,
		bottom: 70,
	}

	drawText(
		hdc,
		"Other applications will not receive typed input.",
		subtitle,
		12,
		400,
		secondary,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	state := Rect{
		left:   width - 158,
		top:    30,
		right:  width - 28,
		bottom: 58,
	}

	fillRect(hdc, state, accentSurface)

	drawText(
		hdc,
		"LOCK ACTIVE",
		state,
		11,
		600,
		accent,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	headerSeparator := Rect{
		left:   28,
		top:    88,
		right:  width - 28,
		bottom: 89,
	}

	fillRect(hdc, headerSeparator, separator)

	statusCard := Rect{
		left:   28,
		top:    108,
		right:  width - 28,
		bottom: 242,
	}

	fillRect(hdc, statusCard, surface)

	cardAccent := statusCard
	cardAccent.bottom = cardAccent.top + 3

	fillRect(hdc, cardAccent, accent)

	metric := Rect{
		left:   46,
		top:    127,
		right:  214,
		bottom: 174,
	}

	drawText(
		hdc,
		fmt.Sprintf("%d", app.keyCount),
		metric,
		27,
		600,
		foreground,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	metricLabel := Rect{
		left:   46,
		top:    174,
		right:  214,
		bottom: 205,
	}

	drawText(
		hdc,
		"KEY PRESSES CAPTURED",
		metricLabel,
		10,
		600,
		quiet,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	cardSeparator := Rect{
		left:   226,
		top:    128,
		right:  227,
		bottom: 222,
	}

	fillRect(hdc, cardSeparator, separator)

	pathLabel := Rect{
		left:   248,
		top:    126,
		right:  width - 48,
		bottom: 150,
	}

	drawText(
		hdc,
		"Capture file",
		pathLabel,
		11,
		600,
		quiet,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	pathArea := Rect{
		left:   248,
		top:    150,
		right:  width - 48,
		bottom: 182,
	}

	drawText(
		hdc,
		app.capturePath,
		pathArea,
		12,
		400,
		foreground,
		dtVCenter|dtSingleLine|dtPathEllipsis|dtNoPrefix,
	)

	fileHint := Rect{
		left:   248,
		top:    188,
		right:  width - 48,
		bottom: 218,
	}

	drawText(
		hdc,
		"Stored locally and opened automatically when you unlock.",
		fileHint,
		11,
		400,
		secondary,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	footerSeparator := Rect{
		left:   28,
		top:    height - 88,
		right:  width - 28,
		bottom: height - 87,
	}

	fillRect(hdc, footerSeparator, separator)

	buttonWidth := min(int32(190), width/2)

	app.button = Rect{
		left:   width - buttonWidth - 28,
		top:    height - 64,
		right:  width - 28,
		bottom: height - 24,
	}

	fillRect(hdc, app.button, button)

	drawText(
		hdc,
		"Unlock keyboard",
		app.button,
		13,
		600,
		buttonText,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	shortcutLabel := Rect{
		left:   28,
		top:    height - 70,
		right:  app.button.left - 16,
		bottom: height - 48,
	}

	drawText(
		hdc,
		"Emergency shortcut",
		shortcutLabel,
		10,
		600,
		quiet,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	shortcut := Rect{
		left:   28,
		top:    height - 48,
		right:  app.button.left - 16,
		bottom: height - 24,
	}

	drawText(
		hdc,
		"Ctrl + Alt + Shift + F12",
		shortcut,
		12,
		600,
		secondary,
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
