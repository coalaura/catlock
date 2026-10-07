//go:build windows

package windows

import (
	"strconv"
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

	markLeft := int32(28)
	markTop := int32(24)

	if app.compact {
		markLeft = 8
		markTop = 8
	}

	mark := Rect{left: markLeft, top: markTop, right: markLeft + 44, bottom: markTop + 44}
	fillRect(hdc, mark, surfaceRaised)

	leftEar := Rect{left: markLeft + 8, top: markTop + 8, right: markLeft + 15, bottom: markTop + 18}
	rightEar := Rect{left: markLeft + 29, top: markTop + 8, right: markLeft + 36, bottom: markTop + 18}
	catHead := Rect{left: markLeft + 8, top: markTop + 15, right: markLeft + 36, bottom: markTop + 36}
	leftEye := Rect{left: markLeft + 14, top: markTop + 22, right: markLeft + 17, bottom: markTop + 25}
	rightEye := Rect{left: markLeft + 27, top: markTop + 22, right: markLeft + 30, bottom: markTop + 25}
	nose := Rect{left: markLeft + 21, top: markTop + 28, right: markLeft + 24, bottom: markTop + 31}

	whiskers := [...]Rect{
		{left: markLeft + 3, top: markTop + 27, right: markLeft + 7, bottom: markTop + 28},
		{left: markLeft + 7, top: markTop + 28, right: markLeft + 12, bottom: markTop + 29},
		{left: markLeft + 3, top: markTop + 33, right: markLeft + 7, bottom: markTop + 34},
		{left: markLeft + 7, top: markTop + 32, right: markLeft + 12, bottom: markTop + 33},
		{left: markLeft + 32, top: markTop + 28, right: markLeft + 37, bottom: markTop + 29},
		{left: markLeft + 37, top: markTop + 27, right: markLeft + 41, bottom: markTop + 28},
		{left: markLeft + 32, top: markTop + 32, right: markLeft + 37, bottom: markTop + 33},
		{left: markLeft + 37, top: markTop + 33, right: markLeft + 41, bottom: markTop + 34},
	}

	fillRect(hdc, leftEar, accent)
	fillRect(hdc, rightEar, accent)
	fillRect(hdc, catHead, accent)
	fillRect(hdc, leftEye, background)
	fillRect(hdc, rightEye, background)
	fillRect(hdc, nose, foreground)

	for _, whisker := range whiskers {
		fillRect(hdc, whisker, accent)
	}

	app.toggleButton = Rect{left: width - 56, top: 30, right: width - 28, bottom: 58}

	if app.compact {
		app.toggleButton = Rect{left: width - 36, top: 16, right: width - 8, bottom: 44}
	}

	fillRect(hdc, app.toggleButton, surfaceRaised)
	fillRect(hdc, Rect{left: app.toggleButton.left + 8, top: app.toggleButton.top + 13, right: app.toggleButton.right - 8, bottom: app.toggleButton.bottom - 13}, foreground)

	if app.compact {
		app.button = Rect{}
		app.logButton = Rect{}

		fillRect(hdc, Rect{left: app.toggleButton.left + 13, top: app.toggleButton.top + 8, right: app.toggleButton.right - 13, bottom: app.toggleButton.bottom - 8}, foreground)

		metric := Rect{left: 60, top: 8, right: width - 44, bottom: 52}
		drawText(hdc, strconv.FormatUint(app.keyCount, 10), metric, 24, 600, foreground, dtVCenter|dtSingleLine|dtNoPrefix)

		return
	}

	title := Rect{
		left:   88,
		top:    20,
		right:  width - 220,
		bottom: 47,
	}

	drawText(
		hdc,
		"Keyboard locked",
		title,
		21,
		600,
		foreground,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	subtitle := Rect{
		left:   88,
		top:    47,
		right:  width - 220,
		bottom: 70,
	}

	drawText(
		hdc,
		"Anything typed now is captured here, not sent to other apps.",
		subtitle,
		12,
		400,
		secondary,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	state := Rect{
		left:   width - 198,
		top:    30,
		right:  width - 68,
		bottom: 58,
	}

	fillRect(hdc, state, accentSurface)

	drawText(
		hdc,
		"CATLOCK ACTIVE",
		state,
		11,
		600,
		accent,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	versionArea := Rect{
		left:   width - 158,
		top:    62,
		right:  width - 28,
		bottom: 78,
	}

	drawText(
		hdc,
		app.version,
		versionArea,
		9,
		400,
		quiet,
		dtRight|dtVCenter|dtSingleLine|dtNoPrefix,
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
		strconv.FormatUint(app.keyCount, 10),
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
		"KEY PRESSES CAUGHT",
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
		"Kept locally and opened with the Release + log button.",
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
	logButtonWidth := min(int32(150), width/4)

	app.button = Rect{
		left:   width - buttonWidth - 28,
		top:    height - 64,
		right:  width - 28,
		bottom: height - 24,
	}

	app.logButton = Rect{
		left:   app.button.left - 12 - logButtonWidth,
		top:    height - 64,
		right:  app.button.left - 12,
		bottom: height - 24,
	}

	fillRect(hdc, app.button, button)
	fillRect(hdc, app.logButton, surfaceRaised)

	drawText(
		hdc,
		"Release keyboard",
		app.button,
		13,
		600,
		buttonText,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	drawText(
		hdc,
		"Release + log",
		app.logButton,
		13,
		600,
		foreground,
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)

	shortcutLabel := Rect{
		left:   28,
		top:    height - 70,
		right:  app.logButton.left - 16,
		bottom: height - 48,
	}

	drawText(
		hdc,
		"Emergency release",
		shortcutLabel,
		10,
		600,
		quiet,
		dtVCenter|dtSingleLine|dtNoPrefix,
	)

	shortcut := Rect{
		left:   28,
		top:    height - 48,
		right:  app.logButton.left - 16,
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
