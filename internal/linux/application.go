//go:build linux

package linux

import (
	"fmt"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	preferredWidth  = 700
	preferredHeight = 350
	preferredMargin = 32
)

type Rect struct {
	left   int
	top    int
	right  int
	bottom int
}

type Point struct {
	x int
	y int
}

type Application struct {
	connection *xgb.Conn
	setup      *xproto.SetupInfo
	screen     *xproto.ScreenInfo
	window     xproto.Window
	gc         xproto.Gcontext

	capture           *CaptureLog
	capturePath       string
	keyCount          uint64
	version           string
	openCaptureOnExit bool

	button    Rect
	logButton Rect
	windowX   int16
	windowY   int16
	width     uint16
	height    uint16
	keyboard  *keyboardMapping
	renderer  *Renderer
	pixels    *pixelEncoder
}

func (area Rect) contains(point Point) bool {
	return point.x >= area.left && point.x < area.right && point.y >= area.top && point.y < area.bottom
}

func (app *Application) runUI() error {
	connection, err := xgb.NewConn()
	if err != nil {
		return fmt.Errorf("connect to X11 display: %w", err)
	}

	app.connection = connection
	defer connection.Close()

	app.setup = xproto.Setup(connection)
	app.screen = app.setup.DefaultScreen(connection)

	if app.screen == nil {
		return fmt.Errorf("find default X11 screen")
	}

	app.refreshMetrics()

	app.renderer, err = newRenderer()
	if err != nil {
		return err
	}

	defer app.renderer.close()

	app.pixels, err = newPixelEncoder(app.setup, app.screen, int(app.width))
	if err != nil {
		return err
	}

	app.keyboard, err = loadKeyboardMapping(connection)
	if err != nil {
		return err
	}

	app.window, err = xproto.NewWindowId(connection)
	if err != nil {
		return fmt.Errorf("allocate X11 window: %w", err)
	}

	eventMask := uint32(
		xproto.EventMaskKeyPress |
			xproto.EventMaskKeyRelease |
			xproto.EventMaskButtonRelease |
			xproto.EventMaskExposure |
			xproto.EventMaskStructureNotify,
	)

	err = xproto.CreateWindowChecked(
		connection,
		app.screen.RootDepth,
		app.window,
		app.screen.Root,
		app.windowX,
		app.windowY,
		app.width,
		app.height,
		0,
		xproto.WindowClassInputOutput,
		app.screen.RootVisual,
		xproto.CwOverrideRedirect|xproto.CwEventMask,
		[]uint32{1, eventMask},
	).Check()

	if err != nil {
		return fmt.Errorf("create X11 window: %w", err)
	}

	defer xproto.DestroyWindow(connection, app.window)

	err = xproto.ChangePropertyChecked(
		connection,
		xproto.PropModeReplace,
		app.window,
		xproto.AtomWmName,
		xproto.AtomString,
		8,
		uint32(len("CatLock")),
		[]byte("CatLock"),
	).Check()

	if err != nil {
		return fmt.Errorf("name X11 window: %w", err)
	}

	app.gc, err = xproto.NewGcontextId(connection)
	if err != nil {
		return fmt.Errorf("allocate X11 graphics context: %w", err)
	}

	err = xproto.CreateGCChecked(connection, app.gc, xproto.Drawable(app.window), 0, nil).Check()
	if err != nil {
		return fmt.Errorf("create X11 graphics context: %w", err)
	}

	defer xproto.FreeGC(connection, app.gc)

	err = xproto.MapWindowChecked(connection, app.window).Check()
	if err != nil {
		return fmt.Errorf("map X11 window: %w", err)
	}

	err = app.raiseWindow()
	if err != nil {
		return err
	}

	err = xproto.SetInputFocusChecked(
		connection,
		xproto.InputFocusPointerRoot,
		app.window,
		xproto.TimeCurrentTime,
	).Check()

	if err != nil {
		return fmt.Errorf("focus X11 window: %w", err)
	}

	grab, err := xproto.GrabKeyboard(
		connection,
		false,
		app.window,
		xproto.TimeCurrentTime,
		xproto.GrabModeAsync,
		xproto.GrabModeAsync,
	).Reply()

	if err != nil {
		return fmt.Errorf("grab X11 keyboard: %w", err)
	}

	if grab == nil || grab.Status != xproto.GrabStatusSuccess {
		status := byte(255)

		if grab != nil {
			status = grab.Status
		}

		return fmt.Errorf("grab X11 keyboard: server returned status %d", status)
	}

	defer xproto.UngrabKeyboard(connection, xproto.TimeCurrentTime)

	err = app.paint()
	if err != nil {
		return err
	}

	keepAboveDone := make(chan struct{})
	keepAboveStopped := make(chan struct{})

	go app.keepAbove(keepAboveDone, keepAboveStopped)

	defer func() {
		close(keepAboveDone)
		<-keepAboveStopped
	}()

	return app.runEvents()
}

func (app *Application) runEvents() error {
	for {
		event, x11Error := app.connection.WaitForEvent()
		if x11Error != nil {
			return fmt.Errorf("wait for X11 event: %v", x11Error)
		}

		if event == nil {
			return fmt.Errorf("X11 connection closed")
		}

		repaint, done, err := app.handleEvent(event)
		if err != nil || done {
			return err
		}

		for {
			event, x11Error = app.connection.PollForEvent()
			if x11Error != nil {
				return fmt.Errorf("poll X11 event: %v", x11Error)
			}

			if event == nil {
				break
			}

			queuedRepaint, queuedDone, queuedErr := app.handleEvent(event)
			repaint = repaint || queuedRepaint

			if queuedErr != nil || queuedDone {
				return queuedErr
			}
		}

		if repaint {
			err = app.paint()
			if err != nil {
				return err
			}
		}
	}
}

func (app *Application) handleEvent(event xgb.Event) (bool, bool, error) {
	switch event := event.(type) {
	case xproto.KeyPressEvent:
		return true, app.captureKey(event), nil
	case xproto.ButtonReleaseEvent:
		point := Point{x: int(event.EventX), y: int(event.EventY)}

		if event.Detail != xproto.ButtonIndex1 {
			return false, false, nil
		}

		if app.button.contains(point) {
			return false, true, nil
		}

		if app.logButton.contains(point) {
			app.openCaptureOnExit = true

			return false, true, nil
		}

		return false, false, nil
	case xproto.ExposeEvent:
		return event.Count == 0, false, nil
	case xproto.MappingNotifyEvent:
		if event.Request != xproto.MappingKeyboard && event.Request != xproto.MappingModifier {
			return false, false, nil
		}

		mapping, err := loadKeyboardMapping(app.connection)
		if err != nil {
			return false, false, err
		}

		app.keyboard = mapping

		return false, false, nil
	case xproto.DestroyNotifyEvent:
		if event.Window == app.window {
			return false, true, nil
		}
	case xproto.UnmapNotifyEvent:
		if event.Window == app.window {
			return false, true, nil
		}
	}

	return false, false, nil
}

func (app *Application) refreshMetrics() {
	screenWidth := max(int(app.screen.WidthInPixels), 1)
	screenHeight := max(int(app.screen.HeightInPixels), 1)

	margin := preferredMargin
	if screenWidth <= 2*margin || screenHeight <= 2*margin {
		margin = 0
	}

	width := min(preferredWidth, screenWidth-2*margin)
	height := min(preferredHeight, screenHeight-2*margin)

	app.width = uint16(max(width, 1))
	app.height = uint16(max(height, 1))
	app.windowX = int16((screenWidth - int(app.width)) / 2)
	app.windowY = int16((screenHeight - int(app.height)) / 2)
}

func (app *Application) raiseWindow() error {
	err := xproto.ConfigureWindowChecked(
		app.connection,
		app.window,
		xproto.ConfigWindowStackMode,
		[]uint32{xproto.StackModeAbove},
	).Check()

	if err != nil {
		return fmt.Errorf("raise X11 window: %w", err)
	}

	return nil
}

func (app *Application) keepAbove(done <-chan struct{}, stopped chan<- struct{}) {
	defer close(stopped)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			xproto.ConfigureWindow(
				app.connection,
				app.window,
				xproto.ConfigWindowStackMode,
				[]uint32{xproto.StackModeAbove},
			)
		}
	}
}
