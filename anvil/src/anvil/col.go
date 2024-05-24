package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/ddkwork/golibrary/mylog"
	"github.com/jeffwilliams/anvil/internal/slice"
	"github.com/jeffwilliams/anvil/internal/typeset"
)

type Col struct {
	Id      int
	Tag     Tag
	Windows []*Window
	LeftX   int // X position of the left of the column

	unpositioned, remove, resized, minimizedExcept, maximize, center []*Window

	maximizedWindow *Window
	layoutBox       layoutBox
	layout          colLayouter
	ed              *Editor

	// vspace is the total vertical space avialable to windows inside this row
	vspace    float32
	Scheduler *Scheduler
	workChan  chan Work
	visible   bool
}

type colLayouter struct {
	layouter
	gtx        layout.Context
	queue      event.Queue
	lineHeight int // Height of a line
	col        *Col
	style      Style
	width      int
	gtxOps
}

func NewCol(style Style) *Col {
	r := &Col{
		layout: colLayouter{
			lineHeight: 20,
			style:      style,
			layouter: layouter{
				fontStyles: style.Fonts,
			},
		},
		visible: true,
	}

	r.Id = application.colIdGenerator.Get()
	r.layoutBox.col = r
	r.setLineHeight()
	r.layout.col = r
	executor := NewCommandExecutor(r)
	finder := NewFileFinder(nil)
	r.Tag.Init(nil, style.tagBlockStyle(), style.tagEditableStyle(), executor, finder, r, r.Scheduler)
	r.Tag.label = "column"
	r.Tag.minHeight = r.layout.lineHeight
	r.layoutBox.Init(style.layoutBoxStyle(), r.layout.lineHeight)
	return r
}

func (r *Col) NewWindow() *Window {
	w := NewWindow(r, r.layout.style)

	// Position the new window
	if len(r.Windows) == 0 {
		w.TopY = 0
		r.Windows = append(r.Windows, w)
	} else {
		// TODO: if there is not enough space fail making this window?
		r.unpositioned = append(r.unpositioned, w)
	}

	return w
}

func (r *Col) NewWindowDontPosition() *Window {
	w := NewWindow(r, r.layout.style)
	r.Windows = append(r.Windows, w)
	return w
}

func (r *Col) setLineHeight() {
	h := mylog.Check2(typeset.CalculateLineHeight(r.layout.curFont(), r.layout.curFontSize(), r.layout.style.LineSpacing))

	r.layout.lineHeight = h.Round()
}

func (c *Col) HandleEvents(gtx layout.Context, queue event.Queue) {
	c.layout.HandleEvents(gtx, queue)
}

func (c *Col) DrawAndListenForEvents(gtx layout.Context, queue event.Queue) {
	dims := c.layout.DrawAndListenForEvents(gtx, queue)
	gtx.Constraints.Max.X = dims.Size.X

	rowHeaderHeight := float32(dims.Size.Y)

	c.vspace = float32(gtx.Constraints.Max.Y) - rowHeaderHeight

	c.positionWindows(rowHeaderHeight)
	c.minimizeOtherWindowsExcept(rowHeaderHeight)
	c.resizeWindows(rowHeaderHeight)
	c.maximizeWindows(rowHeaderHeight)
	c.layout.setOffsetAndLayoutWindows(gtx, queue, dims.Size.Y)
	c.removeWindowsMarkedForRemoval()
	c.centerWindowsMarkedForCentering()
}

func (r *Col) setConstraintsToWindowHeight(gtx *layout.Context, winIndex int) {
	ps := r.asPackables(r.Windows)
	p := NewPacker(0, r.vspace, ps)
	sz := p.ItemSize(winIndex)

	gtx.Constraints.Max.Y = int(sz)
}

func (l *colLayouter) HandleEvents(gtx layout.Context, queue event.Queue) {
	l.gtx = gtx
	l.gtxOps.gtx = gtx
	l.queue = queue

	l.handleLayoutBoxEvents(l.gtx, l.queue)
	l.col.Tag.HandleEvents(gtx, queue)
}

func (l *colLayouter) DrawAndListenForEvents(gtx layout.Context, queue event.Queue) layout.Dimensions {
	l.gtx = gtx
	l.gtxOps.gtx = gtx
	l.queue = queue
	l.width = l.gtx.Constraints.Max.X

	defer l.offset(l.col.LeftX, 0).Pop()

	lboxDims := l.drawLayoutBox(l.gtx, l.queue)

	// Translate all later draw operations so they are to the right of the layoutBox
	defer l.offset(lboxDims.Size.X, 0).Pop()
	l.gtx.Constraints.Max.X -= lboxDims.Size.X
	tagDims := l.col.Tag.DrawAndListenForEvents(l.gtx, queue)
	l.gtx.Constraints.Max.X += lboxDims.Size.X
	defer l.offset(-lboxDims.Size.X, tagDims.Size.Y).Pop()

	// In case the tag takes up multiple lines, color in the part under
	// the layout box
	l.fillUnderLayoutBox(gtx, tagDims.Size.Y-lboxDims.Size.Y)

	// Draw a line (border) under the header
	botBorderHeight := l.drawBottomBorder(l.gtx)

	defer l.offset(0, l.style.WinBorderWidth).Pop()

	l.gtx = layout.Context{}
	l.gtxOps.gtx = layout.Context{}

	return layout.Dimensions{Size: image.Point{X: gtx.Constraints.Max.X, Y: tagDims.Size.Y + botBorderHeight}}
}

func (l *colLayouter) setOffsetAndLayoutWindows(gtx layout.Context, queue event.Queue, startY int) {
	l.gtx = gtx
	l.gtxOps.gtx = gtx
	l.queue = queue
	l.width = l.gtx.Constraints.Max.X

	defer l.offset(l.col.LeftX, 0).Pop()

	st := l.offset(l.gtx.Constraints.Max.X-l.style.WinBorderWidth, 0)
	l.drawRightBorder(l.gtx)
	st.Pop()
	l.gtx.Constraints.Max.X -= l.style.WinBorderWidth

	defer l.offset(0, startY).Pop()

	if len(l.col.Windows) > 0 {
		l.layoutWindows()
	} else {
		l.fillEmptyColumn()
	}
}

func (l *colLayouter) handleLayoutBoxEvents(gtx layout.Context, queue event.Queue) {
	l.col.layoutBox.handleEvents(gtx, queue)
}

func (l *colLayouter) drawLayoutBox(gtx layout.Context, queue event.Queue) layout.Dimensions {
	l.col.layoutBox.dims = l.col.layoutBox.draw(gtx)
	l.col.layoutBox.listenForEvents(gtx)
	return l.col.layoutBox.dims
}

func (l *colLayouter) fillUnderLayoutBox(gtx layout.Context, height int) {
	st := l.offset(0, -height)
	cst := clip.Rect{Max: image.Pt(l.style.GutterWidth, int(height))}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(l.style.BodyBgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	cst.Pop()
	st.Pop()
}

func (l *colLayouter) layoutWindows() {
	lastWindowY := -10000
	for i, w := range l.col.Windows {
		if w.TopY < lastWindowY {
			log(LogCatgCol, "col %p: The windows stored in the row are not sorted in ascending Y coordinate. About to panic. The windows are:\n", l.col)
			l.col.printWindowPositions()
		}
		lastWindowY = w.TopY

		l.col.setConstraintsToWindowHeight(&l.gtx, i)
		w.Layout(l.gtx, l.queue)
	}
}

func (l *colLayouter) fillEmptyColumn() {
	paint.ColorOp{Color: color.NRGBA(l.style.BodyBgColor)}.Add(l.gtx.Ops)
	st := drawFilledBox(l.gtx, float32(l.gtx.Constraints.Max.X), float32(l.gtx.Constraints.Max.Y-l.style.WinBorderWidth))
	paint.PaintOp{}.Add(l.gtx.Ops)
	st.Pop()
}

func (l *colLayouter) drawBottomBorder(gtx layout.Context) (height int) {
	height = l.style.WinBorderWidth
	paint.ColorOp{Color: color.NRGBA(l.style.WinBorderColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), float32(height))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
	return
}

func (l *colLayouter) drawRightBorder(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(l.style.WinBorderColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(l.style.WinBorderWidth), float32(gtx.Constraints.Max.Y))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func (r *Col) positionWindows(rowHeaderHeight float32) {
	if len(r.unpositioned) == 0 {
		return
	}

	for _, w := range r.Windows {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}

	ps := r.asPackables(r.Windows)
	unp := r.asPackables(r.unpositioned)

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	if r.maximizedWindow != nil {
		// Unmaximize all the windows so that this newly added window is packed in with the rest.
		ps = p.MinimizeAllExcept(r.maximizedWindow)
	}
	ps = p.Pack(unp)

	r.setWindowsTo(ps)

	r.unpositioned = nil
}

func (r *Col) resizeWindows(rowHeaderHeight float32) {
	if len(r.resized) == 0 {
		return
	}

	ps := r.asPackables(r.Windows)
	res := r.asPackables(r.resized)
	toCenter := r.copyWindows()

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	amt := r.layout.lineHeight * 10
	for _, r := range res {
		ps = p.Grow(r, float32(amt))
	}

	r.setWindowsTo(ps)

	for _, w := range toCenter {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}

	r.resized = nil
}

func (r *Col) copyWindows() []*Window {
	rc := make([]*Window, len(r.Windows))
	copy(rc, r.Windows)
	return rc
}

func (r *Col) minimizeOtherWindowsExcept(rowHeaderHeight float32) {
	if len(r.minimizedExcept) == 0 {
		return
	}

	ps := r.asPackables(r.Windows)
	res := r.asPackables(r.minimizedExcept)
	toCenter := r.copyWindows()

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	for _, r := range res {
		ps = p.MinimizeAllExcept(r)
	}

	r.setWindowsTo(ps)

	for _, w := range toCenter {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}

	r.minimizedExcept = nil
	r.maximizedWindow = nil
}

func (r *Col) maximizeWindows(rowHeaderHeight float32) {
	if len(r.maximize) == 0 {
		return
	}

	ps := r.asPackables(r.Windows)
	res := r.asPackables(r.maximize)

	p := NewPacker(rowHeaderHeight, r.vspace, ps)

	for _, w := range res {
		ps = p.Maximize(w)
		if win, ok := w.(*Window); ok {
			r.maximizedWindow = (*Window)(win)
		}
	}

	r.setWindowsTo(ps)

	r.maximize = nil
}

func (r *Col) asPackables(a []*Window) []Packable {
	ps := make([]Packable, len(a))
	for i := 0; i < len(a); i++ {
		ps[i] = a[i]
	}
	return ps
}

func (r *Col) setWindowsTo(ps []Packable) {
	for len(r.Windows) < len(ps) {
		r.Windows = append(r.Windows, nil)
	}

	for i := 0; i < len(ps); i++ {
		r.Windows[i] = ps[i].(*Window)
	}
}

func round(f float32) float32 {
	return float32(math.Round(float64(f)))
}

func (r *Col) printWindowPositions() {
	for _, w := range r.Windows {
		log(LogCatgCol, "%p: %d\n", w, w.TopY)
	}
}

func (c *Col) moveWindowBy(w *Window, off f32.Point) {
	absX := off.X + float32(c.LeftX)
	c2 := c.ed.bestColForXCoord(int(absX))
	if c2 != c {
		c.markForRemoval(w)
		w.col = c2
		xDiff := float32(c2.LeftX - c.LeftX)
		c2.moveWindowBy(w, off.Sub(f32.Pt(xDiff, 0)))
		return
	}

	ps := c.asPackables(c.Windows)
	p := NewPacker(float32(w.headerHeight()), c.vspace, ps)
	ps = p.MoveTo(w, float32(w.TopY)+off.Y)

	c.setWindowsTo(ps)
	c.printWindowPositions()
	c.markAllWindowsForCentering()
}

func (r *Col) markForRemoval(w *Window) {
	r.remove = append(r.remove, w)
}

func (r *Col) removeWindowsMarkedForRemoval() {
	if r.remove == nil || len(r.remove) == 0 {
		return
	}

	for _, w := range r.remove {
		r.removeWindow(w)
	}
	r.remove = nil
	if len(r.Windows) > 0 {
		r.Windows[0].TopY = 0
	}
}

func (r *Col) removeWindow(w *Window) {
	match := func(i int) bool {
		return r.unpositioned[i] == w
	}
	r.unpositioned = slice.RemoveFirstMatchFromSlicePreserveOrder(r.unpositioned, match).([]*Window)

	match2 := func(i int) bool {
		return r.Windows[i] == w
	}
	r.Windows = slice.RemoveFirstMatchFromSlicePreserveOrder(r.Windows, match2).([]*Window)

	w.removeFromAllClones()
	// log(LogCatgCol,"col %p: after removal windows are:\n", r)
	// r.printWindowPositions()
	editor.Completer().DeleteAllFromSource(w.Body.completionSource)
	editor.AddRecentFile(w.file)
}

func (c *Col) markForCentering(w *Window) {
	c.center = append(c.center, w)
}

func (c *Col) markAllWindowsForCentering() {
	for _, w := range c.Windows {
		c.markForCentering(w)
	}
}

func (c *Col) centerWindowsMarkedForCentering() {
	for _, w := range c.center {
		w.centerBodyOnFirstCursorOrPrimarySelection()
	}
	c.center = c.center[:0]
}

func (r *Col) PackingCoord() float32 {
	return float32(r.LeftX)
}

func (r *Col) SetPackingCoord(v float32) {
	r.LeftX = int(v)
}

func (r *Col) Clear() {
	for _, w := range r.Windows {
		r.removeWindow(w)
	}
}

func (r *Col) Grow(w *Window) {
	if r.maximizedWindow != nil && len(r.minimizedExcept) == 0 {
		// Allow Growing a window if the user has a pending request that the column
		// show windows minimized except one. This handles a case where the user
		// tried to acquire a window that was invisible while another in the column
		// is maximized
		return
	}
	r.resized = append(r.resized, w)
}

func (r *Col) MinimizeAllExcept(w *Window) {
	r.minimizedExcept = append(r.minimizedExcept, w)
}

func (r *Col) Maximize(w *Window) {
	r.maximize = append(r.maximize, w)
}

func (c *Col) MaximizedWindow() *Window {
	return c.maximizedWindow
}

func (r *Col) Optimize() bool {
	if r.maximizedWindow == nil {
		return false
	}

	r.MinimizeAllExcept(r.maximizedWindow)
	return true
}

func (c *Col) SetStyle(style Style) {
	c.layout.style = style
	c.layout.layouter.fontStyles = style.Fonts
	c.setLineHeight()
	c.Tag.SetStyle(style.tagBlockStyle(), style.tagEditableStyle())
	c.layoutBox.SetStyle(style.layoutBoxStyle(), c.layout.lineHeight)
}

func (c *Col) Visible() bool {
	return c.visible
}

func (c *Col) SetVisible(v bool) {
	if c.visible == v {
		return
	}

	if !v && editor.NumVisibleCols() <= 1 {
		return
	}

	c.visible = v
	if c.visible {
		editor.RepositionCol(c)
		return
	}

	editor.ensureFirstVisibleColIsLeftJustified()
}

func (c *Col) Name() string {
	if c.hasNoUserSetName() {
		return fmt.Sprintf("Col %d", c.Id)
	}

	t := c.Tag.String()
	parts := strings.Split(t, " ")
	return parts[0]
}

func (c *Col) hasNoUserSetName() bool {
	return strings.HasPrefix(c.Tag.String(), "New")
}
