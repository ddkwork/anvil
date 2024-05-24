package main

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"

	"gioui.org/io/event"
	"gioui.org/layout"
	//"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/ddkwork/golibrary/mylog"

	"github.com/jeffwilliams/anvil/internal/events"
)

type scrollbar struct {
	style      scrollbarStyle
	lineHeight int
	dims       layout.Dimensions
	windowBody *Body
	// pressPos     f32.Point
	// window       *Window
	// col          *Col
	// dragging     bool
	pointerState     PointerState
	eventInterceptor *events.EventInterceptor
}

type scrollbarStyle struct {
	FgColor     color.NRGBA
	BgColor     color.NRGBA
	GutterWidth int
}

func (b *scrollbar) Init(style scrollbarStyle, windowBody *Body, lineHeight int) {
	b.style = style
	b.windowBody = windowBody
	b.lineHeight = lineHeight

	b.InitPointerEventHandlers()
}

func (b *scrollbar) InitPointerEventHandlers() {
	b.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonPrimary}, b.moveBackward)
	b.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonSecondary}, b.moveForward)

	b.pointerState.Handler(PointerEventMatch{pointer.Press, pointer.ButtonTertiary}, b.setTextposToMouse)
	b.pointerState.Handler(PointerEventMatch{pointer.Drag, pointer.ButtonTertiary}, b.setTextposToMouse)
	// b.pointerState.Handler(PointerEventMatch{pointer.Release, pointer.ButtonPrimary}, b.onPointerRelease)
}

func (b *scrollbar) layout(gtx layout.Context, queue event.Queue) layout.Dimensions {
	b.handleEvents(gtx, queue)
	b.dims = b.draw(gtx)
	b.listenForEvents(gtx)
	return b.dims
}

func (b *scrollbar) handleEvents(gtx layout.Context, queue event.Queue) {
	for _, ev := range queue.Events(b) {
		switch e := ev.(type) {
		case pointer.Event:
			if b.intercept(gtx, &e) {
				continue
			}
			b.Pointer(gtx, &e)
		}
	}
}

func (b *scrollbar) intercept(gtx layout.Context, ev *pointer.Event) (processed bool) {
	if b.eventInterceptor == nil {
		return false
	}

	return b.eventInterceptor.Filter(gtx, ev)
}

func (b *scrollbar) Pointer(gtx layout.Context, ev *pointer.Event) {
	b.pointerState.currentPointerEvent.set = false
	b.pointerState.Event(ev, gtx)
	b.pointerState.InvokeHandlers()
}

func (b *scrollbar) moveForward(ps *PointerState) {
	b.move(ps, Down)
}

func (b *scrollbar) moveBackward(ps *PointerState) {
	b.move(ps, Up)
}

func (b *scrollbar) move(ps *PointerState, dir verticalDirection) {
	// l.pressPos = ps.currentPointerEvent.Position
	// l.dragging = false
	h := b.windowBody.heightInLines(ps.gtx)
	linesToScroll := lerp(int(ps.currentPointerEvent.Position.Y), ps.gtx.Constraints.Max.Y, h)
	if linesToScroll < 1 {
		linesToScroll = 1
	}

	for ; linesToScroll > 0; linesToScroll-- {
		b.windowBody.ScrollOneLine(ps.gtx, dir)
	}
}

func (b *scrollbar) setTextposToMouse(ps *PointerState) {
	log(LogCatgWin, "drag on scrollbar at %s\n", ps.currentPointerEvent.Position)

	bdy := b.windowBody
	textLen := len(bdy.Bytes())

	targetTextPos := lerp(int(ps.currentPointerEvent.Position.Y), ps.gtx.Constraints.Max.Y, textLen)

	if targetTextPos < 0 {
		targetTextPos = 0
	}

	if targetTextPos > textLen {
		targetTextPos = textLen
	}

	b.windowBody.SetTopLeft(targetTextPos)

	return
}

func (b *scrollbar) draw(gtx layout.Context) layout.Dimensions {
	// Draw a thick bar, then a thin right column
	st := clip.Rect{
		Min: image.Pt(0, 0),
		Max: image.Pt(b.style.GutterWidth, gtx.Constraints.Max.Y),
	}.Push(gtx.Ops)
	paint.ColorOp{Color: b.style.BgColor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	// Draw the button
	top, bot := b.buttonPositions(gtx)
	st = clip.Rect{
		Min: image.Pt(0, top),
		Max: image.Pt(b.style.GutterWidth-1, bot),
	}.Push(gtx.Ops)
	paint.ColorOp{Color: b.style.FgColor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	return layout.Dimensions{Size: image.Point{X: b.style.GutterWidth, Y: gtx.Constraints.Max.Y}}
}

func (b scrollbar) buttonPositions(gtx layout.Context) (top, bottom int) {
	bdy := b.windowBody
	textLen := len(bdy.Bytes())
	r := bdy.TopLeftIndex

	// lh := int(b.lineHeight)//todo test

	top = lerp(r, textLen, gtx.Constraints.Max.Y)

	disp := mylog.Check2(b.lenOfDisplayedBodyTextInBytes(gtx))

	bottom = lerp(r+disp, textLen, gtx.Constraints.Max.Y)

	if bottom-top < 2 {
		bottom = top + 2
	}

	return
}

func (b scrollbar) lenOfDisplayedBodyTextInBytes(gtx layout.Context) (int, error) {
	// When we call LenOfDisplayedTextInBytes on the body of the window, it lays out the
	// text according to the constraints in gtx. At the time of this call, the constraints
	// are set to the size of the entire window; not to the inset remaining portion that is
	// left after rendering the scrollbar. Thus we must set gtx temporarily to the correct width
	gtx.Constraints.Max.X -= b.style.GutterWidth
	bdy := b.windowBody
	disp := mylog.Check2(bdy.LenOfDisplayedTextInBytes(gtx))
	gtx.Constraints.Max.X += b.style.GutterWidth
	return disp, nil
}

// Linear interpolation. Finds the percentage that x is of tot1, and finds that
// percentage of tot2.
func lerp(x, tot1, tot2 int) int {
	if tot1 > 0 {
		return tot2 * x / tot1
	}
	return 0
}

func (b *scrollbar) listenForEvents(gtx layout.Context) {
	r := image.Rectangle{Max: b.dims.Size}
	st := clip.Rect(r).Push(gtx.Ops)

	pointer.InputOp{
		Tag:   b,
		Types: pointer.Press | pointer.Drag | pointer.Release | pointer.Leave,
	}.Add(gtx.Ops)

	st.Pop()
}

func (b *scrollbar) SetStyle(style scrollbarStyle) {
	b.style = style
}
