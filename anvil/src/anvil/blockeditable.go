package main

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// blockEditable is an editable that is displayed as a block with a background
type blockEditable struct {
	editable
	dims      layout.Dimensions
	style     blockStyle
	maximize  bool
	minHeight int
	bgcolor   color.NRGBA
}

type blockStyle struct {
	StandardBgColor   color.NRGBA
	ErrorBgColor      color.NRGBA
	ErrorFlashBgColor color.NRGBA
	PathBasenameColor color.NRGBA
}

func (t *blockEditable) Init(style blockStyle, editableStyle editableStyle, scheduler *Scheduler) {
	t.style = style
	t.editable.Init(editableStyle)
	t.editable.tag = t
	t.editable.Scheduler = scheduler
	t.bgcolor = style.StandardBgColor
}

func (t *blockEditable) layout(gtx layout.Context, queue event.Queue) layout.Dimensions {
	t.HandleEvents(gtx, queue)
	t.DrawAndListenForEvents(gtx, queue)
	return t.dims
}

func (t *blockEditable) HandleEvents(gtx layout.Context, queue event.Queue) {
	t.prepareForLayout()

	for _, ev := range queue.Events(t) {
		switch e := ev.(type) {
		case pointer.Event:
			t.Pointer(gtx, &e)
		case key.Event:
			t.Key(gtx, &e)
		case key.EditEvent:
			t.InsertText(e.Text)
		case key.FocusEvent:
			/*action := "set to"
			  if !e.Focus {
			    action = "cleared from"
			  }
			  log(LogCatgEd,"blockEditable.handleEvents: focus %s %p\n", action, t)*/
			t.FocusChanged(gtx, &e)
		case clipboard.Event:
			t.InsertTextAndSelect(fixLineEndings(e.Text))
		}
	}
}

func (t *blockEditable) DrawAndListenForEvents(gtx layout.Context, queue event.Queue) layout.Dimensions {
	t.relayout(gtx)
	t.dims = t.draw(gtx)
	t.listenForEvents(gtx)
	return t.dims
}

func (t *blockEditable) listenForEvents(gtx layout.Context) {
	r := image.Rectangle{Max: t.dims.Size}
	stack := clip.Rect(r).Push(gtx.Ops)
	defer stack.Pop()

	pointer.InputOp{
		Tag:   t,
		Types: pointer.Press | pointer.Drag | pointer.Release | pointer.Scroll,
		ScrollBounds: image.Rectangle{
			Min: image.Point{-100, -100},
			Max: image.Point{100, 100},
		},
	}.Add(gtx.Ops)

	key.InputOp{
		// Keys: "[←,→,↑,↓,⏎]",
		Keys: t.KeySet(),
		Tag:  t,
	}.Add(gtx.Ops)
}

func (t *blockEditable) draw(gtx layout.Context) layout.Dimensions {
	if t.maximize {
		return t.drawMaximized(gtx)
	} else {
		return t.drawTight(gtx)
	}
}

func (t *blockEditable) drawMaximized(gtx layout.Context) layout.Dimensions {
	r := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}
	stack := r.Push(gtx.Ops)
	defer stack.Pop()
	t.drawBackground(gtx)

	t.editable.draw(gtx)
	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Constraints.Max.Y}}
}

func (t *blockEditable) drawBackground(gtx layout.Context) {
	paint.ColorOp{Color: t.bgcolor}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func (t *blockEditable) drawTight(gtx layout.Context) layout.Dimensions {
	// We don't know how many lines the editable is (how big it is) until we draw it, but we also
	// want to fill the background before drawing the editable. To fill the background we need to know
	// how big it is. So we draw it but record the drawing operations into a macro instead of performing
	// them, then we fill the background and replay the macro.
	macro := op.Record(gtx.Ops)
	dims := t.editable.draw(gtx)
	if t.minHeight > 0 && dims.Size.Y < t.minHeight {
		dims.Size.Y = t.minHeight
	}
	c := macro.Stop()

	// log(LogCatgEd,"blockEditable.drawTight: dimensions for %s are computed to be %#v\n", t.editable.label, tagDimensions)

	r := clip.Rect{Max: image.Pt(dims.Size.X, dims.Size.Y)}
	stack := r.Push(gtx.Ops)
	defer stack.Pop()
	t.drawBackground(gtx)

	c.Add(gtx.Ops)

	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: dims.Size.Y}}
}

func fixLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func (t *blockEditable) SetStyle(style blockStyle, editableStyle editableStyle) {
	t.style = style
	t.editable.SetStyle(editableStyle)
	t.bgcolor = style.StandardBgColor
}
