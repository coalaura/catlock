//go:build linux

package linux

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/unix"
)

type waylandBuffer struct {
	proxy  unsafe.Pointer
	pixels []byte
	width  int
	height int
	busy   bool
}

func (session *waylandSession) newBuffer(width, height int, format uint32) (*waylandBuffer, error) {
	size := width * height * 4

	descriptor, err := unix.MemfdCreate("catlock", unix.MFD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("create Wayland frame buffer: %w", err)
	}

	defer unix.Close(descriptor)

	err = unix.Ftruncate(descriptor, int64(size))
	if err != nil {
		return nil, fmt.Errorf("size Wayland frame buffer: %w", err)
	}

	pixels, err := unix.Mmap(descriptor, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("map Wayland frame buffer: %w", err)
	}

	pool := session.create(session.shm, 0, "wl_shm_pool", 1, 0, waylandArgument(descriptor), waylandArgument(size))
	proxy := session.create(pool, 0, "wl_buffer", 1, 0, 0, waylandArgument(width), waylandArgument(height), waylandArgument(width*4), waylandArgument(format))

	session.destroy(pool, 1)

	buffer := &waylandBuffer{proxy: proxy, pixels: pixels, width: width, height: height}

	session.listen(proxy, func(_ uint32, _ []waylandArgument) {
		buffer.busy = false
	})

	if session.err != nil {
		_ = unix.Munmap(pixels)

		return nil, session.err
	}

	return buffer, nil
}

func (session *waylandSession) paint() error {
	scale := 1

	for output, outputScale := range session.outputs {
		if len(session.entered) == 0 || session.entered[output] {
			scale = max(scale, outputScale)
		}
	}

	width := session.logicalWidth * scale
	height := session.logicalHeight * scale

	if width <= 0 || height <= 0 || width > 65535 || height > 65535 || int64(width)*int64(height) > 16*1024*1024 {
		return fmt.Errorf("the Wayland desktop configured an unsupported window size")
	}

	app := session.app
	if app.renderer == nil || app.uiScale != scale {
		if app.renderer != nil {
			app.renderer.close()
		}

		renderer, err := newRenderer(scale)
		if err != nil {
			app.renderer = nil

			return err
		}

		app.renderer = renderer
		app.uiScale = scale
	}

	if len(session.buffers) != 0 && (session.buffers[0].width != width || session.buffers[0].height != height) {
		for _, buffer := range session.buffers {
			// A destroyed wl_buffer can still be in use by the compositor. Its
			// separate mmap stays valid; these backing files are never reused.
			session.destroy(buffer.proxy, 0)
			_ = unix.Munmap(buffer.pixels)
		}

		session.buffers = session.buffers[:0]
	}

	var available *waylandBuffer

	for _, buffer := range session.buffers {
		if !buffer.busy {
			available = buffer

			break
		}
	}

	if available == nil && len(session.buffers) < 2 {
		buffer, err := session.newBuffer(width, height, 1)
		if err != nil {
			return err
		}

		session.buffers = append(session.buffers, buffer)
		available = buffer
	}

	if available == nil {
		return nil
	}

	app.width = uint16(width)
	app.height = uint16(height)

	frame, err := app.renderer.render(app)
	if err != nil {
		return err
	}

	encodeWaylandFrame(available.pixels, frame)

	session.request(session.surface, 8, waylandArgument(scale))
	session.request(session.surface, 1, waylandPointer(available.proxy), 0, 0)
	session.request(session.surface, 9, 0, 0, waylandArgument(width), waylandArgument(height))
	session.request(session.surface, 6)

	available.busy = true
	session.dirty = false

	return session.err
}

func (session *waylandSession) createCursor() error {
	buffer, err := session.newBuffer(18, 24, 0)
	if err != nil {
		return err
	}

	session.cursorBuffer = buffer
	session.cursor = session.create(session.compositor, 0, "wl_surface", 4, 0)

	for row := range 20 {
		for column := range min(row*2/3+1, 13) {
			value := byte(255)

			if column == 0 || column == row*2/3 || row == 19 {
				value = 0
			}

			offset := (row*18 + column) * 4
			buffer.pixels[offset] = value
			buffer.pixels[offset+1] = value
			buffer.pixels[offset+2] = value
			buffer.pixels[offset+3] = 255
		}
	}

	session.request(session.cursor, 1, waylandPointer(buffer.proxy), 0, 0)
	session.request(session.cursor, 9, 0, 0, 18, 24)
	session.request(session.cursor, 6)

	return session.err
}

func encodeWaylandFrame(destination []byte, frame *image.RGBA) {
	for row := range frame.Rect.Dy() {
		source := frame.Pix[row*frame.Stride : row*frame.Stride+frame.Rect.Dx()*4]
		target := destination[row*frame.Rect.Dx()*4 : (row+1)*frame.Rect.Dx()*4]

		for offset := 0; offset < len(source); offset += 4 {
			// WL_SHM_FORMAT_XRGB8888 is B, G, R, unused on Linux amd64/arm64.
			target[offset] = source[offset+2]
			target[offset+1] = source[offset+1]
			target[offset+2] = source[offset]
			target[offset+3] = 255
		}
	}
}
