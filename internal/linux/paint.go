//go:build linux

package linux

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math/bits"

	"github.com/jezek/xgb/xproto"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type textAlignment uint8

const (
	alignLeft textAlignment = iota
	alignCenter
	alignRight
)

type fontSpec struct {
	size   int
	weight int
}

type Renderer struct {
	regular *opentype.Font
	bold    *opentype.Font
	faces   map[fontSpec]font.Face
}

type pixelEncoder struct {
	bytesPerPixel int
	rowStride     int
	byteOrder     byte
	red           [256]uint32
	green         [256]uint32
	blue          [256]uint32
}

func newRenderer() (*Renderer, error) {
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse regular UI font: %w", err)
	}

	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse bold UI font: %w", err)
	}

	return &Renderer{
		regular: regular,
		bold:    bold,
		faces:   make(map[fontSpec]font.Face),
	}, nil
}

func (renderer *Renderer) close() {
	for _, face := range renderer.faces {
		closer, ok := face.(interface{ Close() error })
		if ok {
			_ = closer.Close()
		}
	}
}

func (renderer *Renderer) face(size, weight int) (font.Face, error) {
	spec := fontSpec{size: size, weight: weight}
	face, ok := renderer.faces[spec]
	if ok {
		return face, nil
	}

	source := renderer.regular

	if weight >= 600 {
		source = renderer.bold
	}

	face, err := opentype.NewFace(source, &opentype.FaceOptions{
		Size:    float64(size) * 72 / 96,
		DPI:     96,
		Hinting: font.HintingFull,
	})

	if err != nil {
		return nil, fmt.Errorf("create %dpx UI font: %w", size, err)
	}

	renderer.faces[spec] = face

	return face, nil
}

func (app *Application) paint() error {
	frame, err := app.renderer.render(app)
	if err != nil {
		return err
	}

	pixels := app.pixels.encode(frame)
	// PutImage pads its data to four bytes after adding the 24-byte header.
	maxPayload := int(app.setup.MaximumRequestLength)*4 - 27
	if maxPayload < app.pixels.rowStride {
		return fmt.Errorf("X11 maximum request is too small for a rendered row")
	}

	rowsPerRequest := maxPayload / app.pixels.rowStride
	height := int(app.height)

	for startRow := 0; startRow < height; startRow += rowsPerRequest {
		rowCount := min(rowsPerRequest, height-startRow)
		start := startRow * app.pixels.rowStride
		end := start + rowCount*app.pixels.rowStride

		err = xproto.PutImageChecked(
			app.connection,
			xproto.ImageFormatZPixmap,
			xproto.Drawable(app.window),
			app.gc,
			app.width,
			uint16(rowCount),
			0,
			int16(startRow),
			0,
			app.screen.RootDepth,
			pixels[start:end],
		).Check()

		if err != nil {
			return fmt.Errorf("paint X11 window: %w", err)
		}
	}

	return nil
}

func (renderer *Renderer) render(app *Application) (*image.RGBA, error) {
	width := int(app.width)
	height := int(app.height)
	frame := image.NewRGBA(image.Rect(0, 0, width, height))

	background := color.RGBA{R: 27, G: 32, B: 39, A: 255}
	surface := color.RGBA{R: 36, G: 43, B: 52, A: 255}
	surfaceRaised := color.RGBA{R: 42, G: 50, B: 60, A: 255}
	border := color.RGBA{R: 73, G: 83, B: 96, A: 255}
	separator := color.RGBA{R: 53, G: 62, B: 73, A: 255}
	foreground := color.RGBA{R: 235, G: 239, B: 243, A: 255}
	secondary := color.RGBA{R: 169, G: 179, B: 190, A: 255}
	quiet := color.RGBA{R: 126, G: 138, B: 151, A: 255}
	accent := color.RGBA{R: 105, G: 172, B: 158, A: 255}
	accentSurface := color.RGBA{R: 38, G: 69, B: 66, A: 255}
	button := color.RGBA{R: 66, G: 111, B: 123, A: 255}
	buttonText := color.RGBA{R: 247, G: 250, B: 251, A: 255}

	fillRect(frame, Rect{left: 0, top: 0, right: width, bottom: height}, border)
	fillRect(frame, Rect{left: 2, top: 2, right: width - 2, bottom: height - 2}, background)
	fillRect(frame, Rect{left: 2, top: 2, right: width - 2, bottom: 5}, accent)
	fillRect(frame, Rect{left: 28, top: 24, right: 72, bottom: 68}, surfaceRaised)

	leftEar := Rect{left: 36, top: 32, right: 43, bottom: 42}
	rightEar := Rect{left: 57, top: 32, right: 64, bottom: 42}
	catHead := Rect{left: 36, top: 39, right: 64, bottom: 60}
	leftEye := Rect{left: 42, top: 46, right: 45, bottom: 49}
	rightEye := Rect{left: 55, top: 46, right: 58, bottom: 49}
	nose := Rect{left: 49, top: 52, right: 52, bottom: 55}

	whiskers := [...]Rect{
		{left: 31, top: 51, right: 35, bottom: 52},
		{left: 35, top: 52, right: 40, bottom: 53},
		{left: 31, top: 57, right: 35, bottom: 58},
		{left: 35, top: 56, right: 40, bottom: 57},
		{left: 60, top: 52, right: 65, bottom: 53},
		{left: 65, top: 51, right: 69, bottom: 52},
		{left: 60, top: 56, right: 65, bottom: 57},
		{left: 65, top: 57, right: 69, bottom: 58},
	}

	fillRect(frame, leftEar, accent)
	fillRect(frame, rightEar, accent)
	fillRect(frame, catHead, accent)
	fillRect(frame, leftEye, background)
	fillRect(frame, rightEye, background)
	fillRect(frame, nose, foreground)

	for _, whisker := range whiskers {
		fillRect(frame, whisker, accent)
	}

	err := renderer.drawText(frame, "Keyboard locked", Rect{left: 88, top: 20, right: width - 180, bottom: 47}, 21, 600, foreground, alignLeft)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, "Anything typed now is captured here, not sent to other apps.", Rect{left: 88, top: 47, right: width - 180, bottom: 70}, 12, 400, secondary, alignLeft)
	if err != nil {
		return nil, err
	}

	state := Rect{left: width - 158, top: 30, right: width - 28, bottom: 58}
	fillRect(frame, state, accentSurface)

	err = renderer.drawText(frame, "CATLOCK ACTIVE", state, 11, 600, accent, alignCenter)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, app.version, Rect{left: width - 158, top: 62, right: width - 28, bottom: 78}, 9, 400, quiet, alignRight)
	if err != nil {
		return nil, err
	}

	fillRect(frame, Rect{left: 28, top: 88, right: width - 28, bottom: 89}, separator)

	statusCard := Rect{left: 28, top: 108, right: width - 28, bottom: 242}
	fillRect(frame, statusCard, surface)
	fillRect(frame, Rect{left: statusCard.left, top: statusCard.top, right: statusCard.right, bottom: statusCard.top + 3}, accent)

	err = renderer.drawText(frame, fmt.Sprintf("%d", app.keyCount), Rect{left: 46, top: 127, right: 214, bottom: 174}, 27, 600, foreground, alignLeft)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, "KEY PRESSES CAUGHT", Rect{left: 46, top: 174, right: 214, bottom: 205}, 10, 600, quiet, alignLeft)
	if err != nil {
		return nil, err
	}

	fillRect(frame, Rect{left: 226, top: 128, right: 227, bottom: 222}, separator)

	err = renderer.drawText(frame, "Capture file", Rect{left: 248, top: 126, right: width - 48, bottom: 150}, 11, 600, quiet, alignLeft)
	if err != nil {
		return nil, err
	}

	pathFace, err := renderer.face(12, 400)
	if err != nil {
		return nil, err
	}

	pathArea := Rect{left: 248, top: 150, right: width - 48, bottom: 182}
	path := fitPath(app.capturePath, pathFace, pathArea.right-pathArea.left)

	err = renderer.drawText(frame, path, pathArea, 12, 400, foreground, alignLeft)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, "Kept locally and opened with the Release + log button.", Rect{left: 248, top: 188, right: width - 48, bottom: 218}, 11, 400, secondary, alignLeft)
	if err != nil {
		return nil, err
	}

	fillRect(frame, Rect{left: 28, top: height - 88, right: width - 28, bottom: height - 87}, separator)

	buttonWidth := min(190, width/2)
	logButtonWidth := min(150, width/4)

	app.button = Rect{left: width - buttonWidth - 28, top: height - 64, right: width - 28, bottom: height - 24}
	app.logButton = Rect{left: app.button.left - 12 - logButtonWidth, top: height - 64, right: app.button.left - 12, bottom: height - 24}

	fillRect(frame, app.button, button)
	fillRect(frame, app.logButton, surfaceRaised)

	err = renderer.drawText(frame, "Release keyboard", app.button, 13, 600, buttonText, alignCenter)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, "Release + log", app.logButton, 13, 600, foreground, alignCenter)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, "Emergency release", Rect{left: 28, top: height - 70, right: app.logButton.left - 16, bottom: height - 48}, 10, 600, quiet, alignLeft)
	if err != nil {
		return nil, err
	}

	err = renderer.drawText(frame, "Ctrl + Alt + Shift + F12", Rect{left: 28, top: height - 48, right: app.logButton.left - 16, bottom: height - 24}, 12, 600, secondary, alignLeft)
	if err != nil {
		return nil, err
	}

	return frame, nil
}

func (renderer *Renderer) drawText(frame *image.RGBA, text string, area Rect, size, weight int, textColor color.RGBA, alignment textAlignment) error {
	if area.right <= area.left || area.bottom <= area.top {
		return nil
	}

	face, err := renderer.face(size, weight)
	if err != nil {
		return err
	}

	textWidth := font.MeasureString(face, text).Ceil()
	x := area.left

	switch alignment {
	case alignCenter:
		x = area.left + (area.right-area.left-textWidth)/2
	case alignRight:
		x = area.right - textWidth
	}

	metrics := face.Metrics()
	textHeight := (metrics.Ascent + metrics.Descent).Ceil()
	baseline := area.top + (area.bottom-area.top-textHeight)/2 + metrics.Ascent.Ceil()
	clip := frame.SubImage(image.Rect(area.left, area.top, area.right, area.bottom)).(draw.Image)
	drawer := font.Drawer{
		Dst:  clip,
		Src:  image.NewUniform(textColor),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}

	drawer.DrawString(text)

	return nil
}

func fillRect(frame *image.RGBA, area Rect, fill color.RGBA) {
	rectangle := image.Rect(area.left, area.top, area.right, area.bottom).Intersect(frame.Bounds())
	if rectangle.Empty() {
		return
	}

	draw.Draw(frame, rectangle, image.NewUniform(fill), image.Point{}, draw.Src)
}

func fitPath(path string, face font.Face, width int) string {
	if width <= 0 || font.MeasureString(face, path).Ceil() <= width {
		return path
	}

	runes := []rune(path)
	if len(runes) < 4 {
		return path
	}

	leftCount := (len(runes) + 1) / 2
	rightStart := leftCount

	for leftCount > 1 && rightStart < len(runes)-1 {
		candidate := string(runes[:leftCount]) + "..." + string(runes[rightStart:])
		if font.MeasureString(face, candidate).Ceil() <= width {
			return candidate
		}

		if leftCount >= len(runes)-rightStart {
			leftCount--
		} else {
			rightStart++
		}
	}

	return "..." + string(runes[len(runes)-1:])
}

func newPixelEncoder(setup *xproto.SetupInfo, screen *xproto.ScreenInfo, width int) (*pixelEncoder, error) {
	var visual *xproto.VisualInfo

	for depthIndex := range screen.AllowedDepths {
		depth := &screen.AllowedDepths[depthIndex]
		if depth.Depth != screen.RootDepth {
			continue
		}

		for visualIndex := range depth.Visuals {
			candidate := &depth.Visuals[visualIndex]
			if candidate.VisualId == screen.RootVisual {
				visual = candidate
				break
			}
		}
	}

	if visual == nil {
		return nil, fmt.Errorf("find X11 root visual")
	}

	if visual.Class != xproto.VisualClassTrueColor {
		return nil, fmt.Errorf("unsupported X11 root visual class %d", visual.Class)
	}

	var format *xproto.Format

	for formatIndex := range setup.PixmapFormats {
		candidate := &setup.PixmapFormats[formatIndex]
		if candidate.Depth == screen.RootDepth {
			format = candidate
			break
		}
	}

	if format == nil {
		return nil, fmt.Errorf("find X11 pixmap format for depth %d", screen.RootDepth)
	}

	if format.BitsPerPixel != 16 && format.BitsPerPixel != 24 && format.BitsPerPixel != 32 {
		return nil, fmt.Errorf("unsupported X11 pixel size %d", format.BitsPerPixel)
	}

	bytesPerPixel := int(format.BitsPerPixel) / 8
	rowBytes := width * bytesPerPixel
	padBytes := max(int(format.ScanlinePad)/8, 1)
	rowStride := (rowBytes + padBytes - 1) / padBytes * padBytes
	encoder := &pixelEncoder{
		bytesPerPixel: bytesPerPixel,
		rowStride:     rowStride,
		byteOrder:     setup.ImageByteOrder,
	}

	for value := range 256 {
		encoder.red[value] = maskedComponent(byte(value), visual.RedMask)
		encoder.green[value] = maskedComponent(byte(value), visual.GreenMask)
		encoder.blue[value] = maskedComponent(byte(value), visual.BlueMask)
	}

	return encoder, nil
}

func (encoder *pixelEncoder) encode(frame *image.RGBA) []byte {
	width := frame.Bounds().Dx()
	height := frame.Bounds().Dy()
	encoded := make([]byte, encoder.rowStride*height)

	for y := range height {
		for x := range width {
			sourceOffset := frame.PixOffset(x, y)
			pixel := encoder.red[frame.Pix[sourceOffset]] |
				encoder.green[frame.Pix[sourceOffset+1]] |
				encoder.blue[frame.Pix[sourceOffset+2]]
			destinationOffset := y*encoder.rowStride + x*encoder.bytesPerPixel

			for byteIndex := 0; byteIndex < encoder.bytesPerPixel; byteIndex++ {
				shift := byteIndex * 8

				if encoder.byteOrder == xproto.ImageOrderMSBFirst {
					shift = (encoder.bytesPerPixel - byteIndex - 1) * 8
				}

				encoded[destinationOffset+byteIndex] = byte(pixel >> shift)
			}
		}
	}

	return encoded
}

func maskedComponent(value byte, mask uint32) uint32 {
	if mask == 0 {
		return 0
	}

	shift := bits.TrailingZeros32(mask)
	componentBits := bits.OnesCount32(mask)
	maximum := uint64(1<<componentBits) - 1
	scaled := (uint64(value)*maximum + 127) / 255

	return uint32(scaled<<shift) & mask
}
