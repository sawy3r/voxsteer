//go:build desktop && linux

package hud

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The HUD is a plain X11 override-redirect window drawn in software with the
// bundled Inter face. Wayland desktops display it through XWayland; when no X
// server is reachable New fails and the application runs without a HUD.

const (
	hudWidth  = 268
	hudHeight = 68

	statusIdle       = 0
	statusRecording  = 1
	statusProcessing = 2
	statusError      = 3

	errorDisplayTime = 5 * time.Second
	paintInterval    = 33 * time.Millisecond
)

// Inter is rendered in-process; it is not installed into the user's fonts.
//
//go:embed assets/Inter-Regular.ttf
var interRegularFont []byte

var (
	backgroundColor = color.RGBA{R: 13, G: 22, B: 38, A: 255}
	idleTextColor   = color.RGBA{R: 185, G: 198, B: 218, A: 255}
	recordingColor  = color.RGBA{R: 42, G: 226, B: 231, A: 255}
	processingColor = color.RGBA{R: 116, G: 205, B: 255, A: 255}
	errorTextColor  = color.RGBA{R: 251, G: 146, B: 60, A: 255}
	waveColor       = color.RGBA{R: 34, G: 211, B: 238, A: 255}
)

type commandKind int

const (
	cmdStatus commandKind = iota
	cmdLevel
	cmdError
	cmdErrorTimeout
	cmdExpose
	cmdClose
)

type command struct {
	kind   commandKind
	status int
	level  float32
}

type Overlay struct {
	commands chan command
	done     chan struct{}
}

func New() (*Overlay, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect to X server: %w", err)
	}
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	parsedFont, err := opentype.Parse(interRegularFont)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("parse Inter font: %w", err)
	}
	face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{
		Size:    12,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create Inter HUD face: %w", err)
	}

	window, err := xproto.NewWindowId(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	x, y := hudPosition(conn, screen)
	backPixel := uint32(backgroundColor.R)<<16 |
		uint32(backgroundColor.G)<<8 | uint32(backgroundColor.B)
	createErr := xproto.CreateWindowChecked(
		conn,
		screen.RootDepth,
		window,
		screen.Root,
		x,
		y,
		hudWidth,
		hudHeight,
		0,
		xproto.WindowClassInputOutput,
		screen.RootVisual,
		xproto.CwBackPixel|xproto.CwOverrideRedirect|xproto.CwEventMask,
		[]uint32{backPixel, 1, xproto.EventMaskExposure},
	).Check()
	if createErr != nil {
		conn.Close()
		return nil, fmt.Errorf("create recording HUD: %w", createErr)
	}

	gc, err := xproto.NewGcontextId(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := xproto.CreateGCChecked(
		conn,
		gc,
		xproto.Drawable(window),
		xproto.GcGraphicsExposures,
		[]uint32{0},
	).Check(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("create HUD graphics context: %w", err)
	}

	overlay := &Overlay{
		commands: make(chan command, 16),
		done:     make(chan struct{}),
	}
	go overlay.pumpEvents(conn)
	go overlay.run(conn, setup, screen, window, gc, face, x, y)
	return overlay, nil
}

func (o *Overlay) SetStatus(status string) {
	value := statusIdle
	switch status {
	case "recording":
		value = statusRecording
	case "transcribing":
		value = statusProcessing
	}
	o.send(command{kind: cmdStatus, status: value})
}

func (o *Overlay) SetLevel(level float32) {
	if o == nil {
		return
	}
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	// Levels arrive at the audio callback rate; dropping one is harmless.
	select {
	case o.commands <- command{kind: cmdLevel, level: level}:
	case <-o.done:
	default:
	}
}

func (o *Overlay) SetError() {
	o.send(command{kind: cmdError})
}

func (o *Overlay) Close() {
	o.send(command{kind: cmdClose})
}

func (o *Overlay) send(c command) {
	if o == nil {
		return
	}
	select {
	case o.commands <- c:
	case <-o.done:
	}
}

func (o *Overlay) pumpEvents(conn *xgb.Conn) {
	for {
		event, err := conn.WaitForEvent()
		if event == nil && err == nil {
			return
		}
		if err != nil {
			continue
		}
		if _, ok := event.(xproto.ExposeEvent); ok {
			select {
			case o.commands <- command{kind: cmdExpose}:
			case <-o.done:
				return
			default:
			}
		}
	}
}

func (o *Overlay) run(
	conn *xgb.Conn,
	setup *xproto.SetupInfo,
	screen *xproto.ScreenInfo,
	window xproto.Window,
	gc xproto.Gcontext,
	face font.Face,
	x, y int16,
) {
	status := statusIdle
	visible := false
	dirty := false
	var history [24]float32
	var errorTimer *time.Timer

	frame := image.NewRGBA(image.Rect(0, 0, hudWidth, hudHeight))
	pixels := make([]byte, hudWidth*hudHeight*4)

	stopErrorTimer := func() {
		if errorTimer != nil {
			errorTimer.Stop()
			errorTimer = nil
		}
	}
	show := func() {
		xproto.ConfigureWindow(
			conn,
			window,
			xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowStackMode,
			[]uint32{uint32(uint16(x)), uint32(uint16(y)), xproto.StackModeAbove},
		)
		xproto.MapWindow(conn, window)
		visible = true
		dirty = true
	}
	hide := func() {
		if visible {
			xproto.UnmapWindow(conn, window)
			visible = false
		}
	}
	paint := func() {
		renderFrame(frame, face, status, history)
		encodePixels(pixels, frame, setup.ImageByteOrder == xproto.ImageOrderMSBFirst)
		xproto.PutImage(
			conn,
			xproto.ImageFormatZPixmap,
			xproto.Drawable(window),
			gc,
			hudWidth,
			hudHeight,
			0,
			0,
			0,
			screen.RootDepth,
			pixels,
		)
		dirty = false
	}

	ticker := time.NewTicker(paintInterval)
	defer ticker.Stop()

	for {
		select {
		case c := <-o.commands:
			switch c.kind {
			case cmdStatus:
				stopErrorTimer()
				status = c.status
				if status == statusRecording {
					history = [24]float32{}
					show()
				} else if status == statusIdle {
					hide()
				}
				dirty = true
			case cmdLevel:
				copy(history[:len(history)-1], history[1:])
				history[len(history)-1] = c.level
				dirty = true
			case cmdError:
				stopErrorTimer()
				status = statusError
				show()
				errorTimer = time.AfterFunc(errorDisplayTime, func() {
					o.send(command{kind: cmdErrorTimeout})
				})
			case cmdErrorTimeout:
				if status == statusError {
					status = statusIdle
					hide()
				}
			case cmdExpose:
				if visible {
					paint()
				}
			case cmdClose:
				stopErrorTimer()
				hide()
				close(o.done)
				conn.Close()
				return
			}
		case <-ticker.C:
			if visible && dirty {
				paint()
			}
		}
	}
}

// hudPosition centers the HUD near the top of the primary monitor, falling
// back to the X screen when RandR is unavailable.
func hudPosition(conn *xgb.Conn, screen *xproto.ScreenInfo) (int16, int16) {
	monX, monY := 0, 0
	monWidth := int(screen.WidthInPixels)

	if err := randr.Init(conn); err == nil {
		if primary, err := randr.GetOutputPrimary(conn, screen.Root).Reply(); err == nil && primary.Output != 0 {
			if info, err := randr.GetOutputInfo(conn, primary.Output, 0).Reply(); err == nil && info.Crtc != 0 {
				if crtc, err := randr.GetCrtcInfo(conn, info.Crtc, 0).Reply(); err == nil && crtc.Width > 0 {
					monX, monY = int(crtc.X), int(crtc.Y)
					monWidth = int(crtc.Width)
				}
			}
		}
	}
	return int16(monX + (monWidth-hudWidth)/2), int16(monY + 48)
}

func renderFrame(frame *image.RGBA, face font.Face, status int, history [24]float32) {
	draw.Draw(frame, frame.Bounds(), image.NewUniform(backgroundColor), image.Point{}, draw.Src)

	label := "VoxSteer ready"
	labelColor := idleTextColor
	switch status {
	case statusRecording:
		label = "Listening"
		labelColor = recordingColor
	case statusProcessing:
		label = "Transcribing..."
		labelColor = processingColor
	case statusError:
		label = "Error - see log"
		labelColor = errorTextColor
	}
	drawer := font.Drawer{
		Dst:  frame,
		Src:  image.NewUniform(labelColor),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I(hudTextLeft),
			Y: fixed.I(hudTextTop) + face.Metrics().Ascent,
		},
	}
	drawer.DrawString(label)

	waveRight := hudWidth - 16
	waveStep, waveVisible := waveformLayout(waveRight, len(history))
	if !waveVisible {
		return
	}
	waveSource := image.NewUniform(waveColor)
	for i, value := range history {
		switch status {
		case statusProcessing:
			value = float32((i%4)+2) / 6
		case statusError:
			value = 0.18
		case statusIdle:
			value = 0.12
		}
		height := int(4 + value*18)
		left := waveformLeft + i*waveStep
		bar := image.Rect(
			left,
			waveformCenter-height/2,
			left+waveformBarWidth,
			waveformCenter+height/2,
		)
		draw.Draw(frame, bar, waveSource, image.Point{}, draw.Src)
	}
}

// encodePixels converts the RGBA frame to a 32-bit ZPixmap in the X server's
// byte order.
func encodePixels(dst []byte, frame *image.RGBA, msbFirst bool) {
	src := frame.Pix
	if msbFirst {
		for i := 0; i+3 < len(src); i += 4 {
			dst[i] = 0
			dst[i+1] = src[i]
			dst[i+2] = src[i+1]
			dst[i+3] = src[i+2]
		}
		return
	}
	for i := 0; i+3 < len(src); i += 4 {
		dst[i] = src[i+2]
		dst[i+1] = src[i+1]
		dst[i+2] = src[i]
		dst[i+3] = 0
	}
}
