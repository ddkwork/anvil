package main

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"

	"gioui.org/f32"

	"gioui.org/io/event"
	"gioui.org/layout"

	//"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/jeffwilliams/anvil/internal/events"
)

type layoutBox struct {
	height                    int
	style                     layoutBoxStyle
	dims                      layout.Dimensions
	pressPos                  f32.Point
	window                    *Window
	col                       *Col
	dragging                  bool
	pointerState              PointerState
	lastGrowPointerEventPress *pointer.Event
	lastGrowYOffset           int
	eventInterceptor          *events.EventInterceptor
}

type layoutBoxStyle struct {
	FgColor        color.NRGBA
	BgColor        color.NRGBA
	UnsavedBgColor color.NRGBA
	GutterWidth    int
}

func (l *layoutBox) Init(style layoutBoxStyle, height int) {
	l.style = style
	l.height = height

	l.InitPointerEventHandlers()
}

func (l *layoutBox) InitPointerEventHandlers() {
	// Clicks
	l.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonPrimary}, l.onPointerPrimaryButtonPress)
	l.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonSecondary}, l.onPointerSecondaryButtonPress)
	l.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonTertiary}, l.onPointerTertiaryButtonPress)

	l.pointerState.Handler(PointerEventMatch{pointer.Drag, pointer.ButtonPrimary}, l.onPointerDrag)
	l.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonPrimary}, l.onPointerRelease)
}

func (l *layoutBox) layout(gtx layout.Context, queue event.Queue) layout.Dimensions {
	l.handleEvents(gtx, queue)
	l.dims = l.draw(gtx)
	l.listenForEvents(gtx)
	return l.dims
}

func (l *layoutBox) handleEvents(gtx layout.Context, queue event.Queue) {
	for _, ev := range queue.Events(l) {
		switch e := ev.(type) {
		case pointer.Event:
			if l.intercept(gtx, &e) {
				continue
			}
			l.Pointer(gtx, &e)
		}
	}
}

func (l *layoutBox) Pointer(gtx layout.Context, ev *pointer.Event) {
	l.pointerState.currentPointerEvent.set = false
	l.pointerState.Event(ev, gtx)
	l.pointerState.InvokeHandlers()
}

func (l *layoutBox) onPointerPrimaryButtonPress(ps *PointerState) {
	log(LogCatgWin, "primary button press on layout box at %s\n", ps.currentPointerEvent.Position)
	l.pressPos = ps.currentPointerEvent.Position
	l.dragging = false
}

func (l *layoutBox) onPointerSecondaryButtonPress(ps *PointerState) {
	log(LogCatgWin, "secondary button press on layout box at %s\n", ps.currentPointerEvent.Position)
	if l.window == nil || l.window.col == nil {
		return
	}
	l.window.col.MinimizeAllExcept(l.window)
	l.dragging = false
}

func (l *layoutBox) onPointerTertiaryButtonPress(ps *PointerState) {
	log(LogCatgWin, "tertiary button press on layout box at %s\n", ps.currentPointerEvent.Position)
	if l.window == nil || l.window.col == nil {
		return
	}
	l.window.col.Maximize(l.window)
	l.dragging = false
}

func (l *layoutBox) onPointerDrag(ps *PointerState) {
	// setCursor("icon")
	l.dragging = true
}

func (l *layoutBox) onPointerRelease(ps *PointerState) {
	log(LogCatgWin, "button release for %s on layout box. col: %p, window %p\n", ps.currentPointerEvent.Buttons, l.col, l.window)
	if l.dragging {
		// For some reason button release doesn't indicate which button was released...
		if l.pressPos.Y != ps.currentPointerEvent.Position.Y {
			if l.window != nil {
				// Move the window

				// The pointer click coordinates are relative to the current draw transformation, which
				// happens to mean relative to the top left of the layout box (not the screen)
				l.window.col.moveWindowBy(l.window, ps.currentPointerEvent.Position.Sub(l.pressPos))
			} else {
				// This is a layout box of a column. Move the column
				l.col.ed.moveColBy(l.col, ps.currentPointerEvent.Position.Sub(l.pressPos))
			}
		}
		l.dragging = false
	} else {
		l.maybeGrow(&ps.currentPointerEvent.Event)
	}
	// setCursor("arrow")
}

func (l *layoutBox) maybeGrow(e *pointer.Event) {
	if l.window == nil || l.window.col == nil {
		return
	}

	windowWasMaximized := l.window.col.Optimize()
	if !windowWasMaximized {
		l.window.col.Grow(l.window)
		l.lastGrowPointerEventPress = &pointer.Event{}
		*l.lastGrowPointerEventPress = *e
	}
}

func (l *layoutBox) draw(gtx layout.Context) layout.Dimensions {
	// stack := op.Save(gtx.Ops)
	// defer stack.Load()

	// Append operation to set the clip region to a rectangle
	st := clip.Rect{Max: image.Pt(l.style.GutterWidth, int(l.height))}.Push(gtx.Ops)
	paint.ColorOp{Color: l.bgColor()}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	st2 := drawBox(gtx, float32(l.style.GutterWidth), float32(l.height), 2)
	// Append operation to set the pen color
	paint.ColorOp{Color: l.style.FgColor}.Add(gtx.Ops)
	// Append the operation that fills the clip region with the pen
	paint.PaintOp{}.Add(gtx.Ops)
	st2.Pop()
	return layout.Dimensions{Size: image.Point{X: l.style.GutterWidth, Y: int(l.height)}}
}

func (l *layoutBox) bgColor() color.NRGBA {
	bgColor := l.style.BgColor
	if l.window != nil && l.window.bodyChangedFromDisk() && !l.window.IsErrorsWindow() && l.window.fileType != typeDir {
		bgColor = l.style.UnsavedBgColor
	}
	return bgColor
}

func (l *layoutBox) listenForEvents(gtx layout.Context) {
	// stack := op.Save(gtx.Ops)
	// defer stack.Load()

	r := image.Rectangle{Max: l.dims.Size}
	st := clip.Rect(r).Push(gtx.Ops)

	pointer.InputOp{
		Tag:   l,
		Types: pointer.Press | pointer.Drag | pointer.Release,
	}.Add(gtx.Ops)

	st.Pop()
}

func (l *layoutBox) WindowPackingCoordChanged(old, new int) {
	l.lastGrowYOffset = old - new
}

func (l *layoutBox) InterceptEvent(gtx layout.Context, ev event.Event) (processed bool) {
	// This function is used to handle a special case. When the user left-clicks on a layout box it
	// grows the window a little. We want it so that if the user doesn't move the mouse at all and
	// clicks in exactly the same place it grows again, simulating the same behaviour of Acme. In Acme's
	// case after the first click it warps the pointer into the new position of the layout box. We don't
	// have that luxury with Gio.
	//
	// Instead, we hack. If the layout box has moved upwards because the window grew, then the
	// mouse would now be inside the scrollbar of the window. So when a pointer click occurs on the scrollbar
	// we have it first check with us to see if we should instead process it as a grow.

	if l.lastGrowPointerEventPress == nil {
		log(LogCatgWin, "layoutBox.InterceptEvent: lastGrowPointerEventPress is nil\n")
		return
	}

	pe, ok := ev.(*pointer.Event)
	if !ok {
		log(LogCatgWin, "layoutBox.InterceptEvent: not a pointer event (type is %T)\n", ev)
		l.lastGrowPointerEventPress = nil
		return
	}

	if pe.Type != pointer.Press {
		log(LogCatgWin, "layoutBox.InterceptEvent: not a pointer press, it is a %s\n", pe.Type)
		if pe.Type == pointer.Leave || (pe.Type != pointer.Release && pe.Type != pointer.Drag) {
			// if pe.Type != pointer.Release && pe.Type != pointer.Drag {
			l.lastGrowPointerEventPress = nil
		}
		return
	}

	if pe.Buttons != pointer.ButtonPrimary {
		log(LogCatgWin, "layoutBox.InterceptEvent: buttons are not just ButtonPrimary\n")
		l.lastGrowPointerEventPress = nil
		return
	}

	log(LogCatgWin, "layoutBox.InterceptEvent: pe.Position=%s lastGrowYOffset=%d lastGrowPressPosition=%s height=%d\n", pe.Position, l.lastGrowYOffset, l.lastGrowPointerEventPress.Position, l.height)
	a := pe.Position
	a.Y = a.Y - float32(l.lastGrowYOffset) + float32(l.height)
	b := l.lastGrowPointerEventPress.Position

	if !pointsAreAlmostSame(&a, &b, pointerEventDistanceToleranceSquared) {
		log(LogCatgWin, "layoutBox.InterceptEvent: events are not in the same place. a=%s and b=%s\n", a, b)
		l.lastGrowPointerEventPress = nil
		return
	}

	log(LogCatgWin, "layoutBox.InterceptEvent: This is for us! Process it\n")
	// l.Pointer(gtx, pe)
	l.maybeGrow(pe)
	l.lastGrowPointerEventPress.Position.Y += float32(l.height)

	processed = true
	return
}

func (l *layoutBox) SetStyle(style layoutBoxStyle, height int) {
	l.style = style
	l.height = height
}

func (l *layoutBox) intercept(gtx layout.Context, ev *pointer.Event) (processed bool) {
	if l.eventInterceptor == nil {
		return false
	}

	return l.eventInterceptor.Filter(gtx, ev)
}
