package main

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/ddkwork/golibrary/mylog"

	"github.com/jeffwilliams/anvil/internal/events"
	"github.com/jeffwilliams/anvil/internal/typeset"
)

// Window is a single window in the editor, with it's own tag and body.
type Window struct {
	Tag  Tag
	Body Body
	TopY int // Y position of the top of the window within the column
	Id   int

	layoutBox layoutBox
	scrollbar scrollbar

	layout                        windowLayouter
	overlayWithGrey               bool
	col                           *Col
	file                          string
	fileType                      fileType
	filler                        *FillEditableWithItemList
	initialTagUserArea            string
	setFocusOnNextLayout          bool
	tagShowsBodyAsChangedFromDisk bool
	bodyDims                      layout.Dimensions
	clones                        map[*Window]struct{}
	allowDirtyDelete              bool
	packingCoordChangedListeners  []func(oldVal, newVal int)
	customEdCommands              string
}

type fileType int

const (
	typeUnknown fileType = iota
	typeFile
	typeDir
)

type windowLayouter struct {
	layouter
	gtx        layout.Context
	queue      event.Queue
	lineHeight int // Height of a line
	window     *Window
	style      Style

	// Temporary variable used to control later text drawing operations
	fgColor *color.NRGBA
}

func NewWindow(row *Col, style Style) *Window {
	w := &Window{
		layout: windowLayouter{
			lineHeight: 20,
			style:      style,
			layouter: layouter{
				fontStyles: style.Fonts,
			},
		},
		col: row,
	}

	w.Id = application.winIdGenerator.Get()
	w.layoutBox.window = w
	w.setLineHeight()
	w.layout.window = w
	executor := NewCommandExecutor(w)
	finder := NewFileFinder(w)
	w.initialTagUserArea = settings.Layout.WindowTagUserArea
	w.Tag.Init(&w.Body, style.tagBlockStyle(), style.tagEditableStyle(), executor, finder, w, row.Scheduler)
	w.Tag.minHeight = w.layout.lineHeight
	w.Body.Init(style.bodyBlockStyle(), style.bodyEditableStyle(), style.Syntax, executor, finder, w, row.workChan)
	w.layoutBox.Init(style.layoutBoxStyle(), w.layout.lineHeight)
	w.scrollbar.Init(style.scrollbarStyle(), &w.Body, w.layout.lineHeight)
	w.Body.AddTextChangeListener(w.redrawClonesOnTextChange)
	w.Body.AddTextChangeListener(w.disallowDirtyDelete)
	w.Body.AddTextChangeListener(w.notifyApiBodyChanged)
	w.setupInterception()
	w.AddPackingCoordChangeListener(w.layoutBox.WindowPackingCoordChanged)
	w.Body.completer = editor.Completer()
	return w
}

func (w *Window) setupInterception() {
	interceptor := &events.EventInterceptor{}
	w.scrollbar.eventInterceptor = interceptor
	interceptor.RegisterInterceptor(&w.layoutBox)
	interceptor.RegisterInterceptor(w)

	interceptor = &events.EventInterceptor{}
	w.layoutBox.eventInterceptor = interceptor
	interceptor.RegisterInterceptor(w)
}

func (w *Window) setLineHeight() {
	h := mylog.Check2(typeset.CalculateLineHeight(w.layout.curFont(), w.layout.curFontSize(), w.layout.style.LineSpacing))

	w.layout.lineHeight = h.Round()
}

func (c *Window) SetFocus(gtx layout.Context) {
	c.Body.AddOpForNextLayout(func(gtx layout.Context) {
		c.Body.SetFocus(gtx)
	})
}

func (c *Window) headerHeight() int {
	return c.layout.lineHeight
}

func (c *Window) PackingCoord() float32 {
	return float32(c.TopY)
}

func (c *Window) SetPackingCoord(v float32) {
	old := c.TopY
	c.TopY = int(v)

	for _, l := range c.packingCoordChangedListeners {
		l(old, c.TopY)
	}
}

func (c *Window) AddPackingCoordChangeListener(f func(oldVal, newVal int)) {
	c.packingCoordChangedListeners = append(c.packingCoordChangedListeners, f)
}

// Layout handles events and draws the window.
// The window is drawn as large as gtx.Constraints.Max allows.
// TODO: the row layout should pass the right constraints
func (c *Window) Layout(gtx layout.Context, queue event.Queue) layout.Dimensions {
	// log(LogCatgWin,"Window.Layout: window %s: body marked at start: %v\n",
	//	c.file,
	//	c.Body.text.IsMarked())

	c.layout.layout(gtx, queue)

	// In case the Tag's file has changed, update our file from it.
	c.UpdateFilenameFromTag()

	if c.tagShowsBodyAsChangedFromDisk != c.bodyChangedFromDisk() {
		c.SetTag()
	}
	c.tagShowsBodyAsChangedFromDisk = c.bodyChangedFromDisk()

	// Window takes up all available space.
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (w *Window) bodyChangedFromDisk() bool {
	return !w.Body.text.IsMarked()
}

func (l *windowLayouter) layout(gtx layout.Context, queue event.Queue) {
	l.gtx = gtx
	l.queue = queue

	wholeStack := op.Offset(image.Point{0, l.window.TopY}).Push(gtx.Ops)
	originalConstraints := gtx.Constraints

	// Draw the lefthand scrollbar and little movement box
	gutterDims := l.layoutGutter(gtx, queue)

	// Translate all later draw operations so they are to the right of the gutter
	gtx.Constraints.Max.X = gtx.Constraints.Max.X - gutterDims.Size.X
	windowStack := op.Offset(image.Point{gutterDims.Size.X, 0}).Push(gtx.Ops)

	tagDims := l.window.Tag.layout(gtx, queue)

	// Translate all later draw operations so they are below the tag
	gtx.Constraints.Max.Y = gtx.Constraints.Max.Y - tagDims.Size.Y
	op.Offset(image.Point{0, tagDims.Size.Y}).Add(gtx.Ops)
	l.window.bodyDims = l.window.Body.layout(gtx, queue)

	// Draw a line (border) at the bottom of the window
	op.Offset(image.Point{0, gtx.Constraints.Max.Y - l.style.WinBorderWidth}).Add(gtx.Ops)
	gtx.Constraints.Max.Y = gtx.Constraints.Max.Y - l.style.WinBorderWidth

	// Undo the translation pushing things to the right of the gutter
	gtx.Constraints.Max.X = gtx.Constraints.Max.X + gutterDims.Size.X
	op.Offset(image.Point{-gutterDims.Size.X, 0}).Add(gtx.Ops)

	// Already saves clip/transfor state
	l.drawBottomBorder(gtx)

	windowStack.Pop()

	l.overlayWithGrey(gtx, originalConstraints)

	wholeStack.Pop()

	l.gtx = layout.Context{}
}

func (l *windowLayouter) overlayWithGrey(gtx layout.Context, originalConstraints layout.Constraints) {
	if !l.window.overlayWithGrey {
		return
	}

	st := clip.Rect{
		Min: image.Pt(0, 0),
		Max: image.Pt(originalConstraints.Max.X, originalConstraints.Max.Y),
	}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA{R: 0xc0, G: 0xc0, B: 0xc0, A: 0x80}}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func (l *windowLayouter) layoutGutter(gtx layout.Context, queue event.Queue) layout.Dimensions {
	l.window.layoutBox.layout(gtx, queue)

	// Translate a bit vertically to draw the scrollbar below the layoutBox
	st := op.Offset(image.Point{0, l.window.layoutBox.height}).Push(gtx.Ops)
	l.window.scrollbar.layout(gtx, queue)

	st.Pop()

	return layout.Dimensions{Size: image.Point{X: l.style.GutterWidth, Y: gtx.Constraints.Max.Y}}
}

func (l *windowLayouter) drawBottomBorder(gtx layout.Context) {
	paint.ColorOp{Color: color.NRGBA(l.style.WinBorderColor)}.Add(gtx.Ops)
	st := drawFilledBox(gtx, float32(gtx.Constraints.Max.X), float32(l.style.WinBorderWidth))
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func (l *windowLayouter) drawLayoutBox(gtx layout.Context) {
	l.window.layoutBox.draw(gtx)
}

func (l *windowLayouter) drawScrollbar(gtx layout.Context) {
	lh := int(l.lineHeight)

	// Draw a thick bar, then a thin right column
	st := clip.Rect{
		Min: image.Pt(0, lh),
		Max: image.Pt(l.style.GutterWidth, gtx.Constraints.Max.Y),
	}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(l.style.ScrollBgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()

	bdy := l.window.Body
	textLen := len(bdy.Bytes())
	r := bdy.TopLeftIndex

	dist := 0
	if textLen > 0 {
		dist = (gtx.Constraints.Max.Y - lh) * r / textLen
	}

	disp := mylog.Check2(bdy.LenOfDisplayedTextInBytes(gtx))

	end := 0
	if textLen > 0 {
		end = (gtx.Constraints.Max.Y - lh) * (r + disp) / textLen
	}

	// Draw the button
	st = clip.Rect{
		Min: image.Pt(0, lh+dist),
		Max: image.Pt(l.style.GutterWidth-1, lh+end),
	}.Push(gtx.Ops)
	// Max: image.Pt(l.style.GutterWidth-1, lh+lh+dist)}.Push(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA(l.style.ScrollFgColor)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	st.Pop()
}

func removeFirstNRunes(b []byte, n int) []byte {
	for ; n > 0; n-- {
		_, size := utf8.DecodeRune(b)
		b = b[size:]
	}
	return b
}

func firstNRunes(b []byte, n int) (first, rest []byte, runeCount int) {
	off := 0
	for ; n > 0 && off < len(b); n-- {
		_, size := utf8.DecodeRune(b[off:])
		off += size
		runeCount++
	}
	first = b[0:off]
	rest = b[off:]
	return
}

func firstNRunesStr(s string, n int) (first, rest string, runeCount int) {
	b := []byte(s)
	fb, rb, runeCount := firstNRunes(b, n)
	first = string(fb)
	rest = string(rb)
	return
}

func (c *Window) SetTag() {
	var t string

	if c.customEdCommandsSet() {
		t = c.customEdCommands
	} else if c.IsErrorsWindow() {
		t = c.edCommandsForErrorsWindow()
	} else if c.fileType == typeFile {
		t = c.edCommandsForFile()
	} else {
		t = c.edCommandsForDir()
	}

	userArea := mylog.Check2(c.userArea(c.file))
	t = t
	userArea = userArea
	c.Tag.label = fmt.Sprintf("tag of %s", c.file)
	c.Body.label = fmt.Sprintf("body of %s", c.file)
}

func (c *Window) edCommandsForFile() string {
	// log(LogCatgWin,"Window.fileTag: body marked: %v\n", c.Body.text.IsMarked())
	put := ""
	if !c.Body.text.IsMarked() {
		put = "Put"
	}
	return fmt.Sprintf(" Del Snarf %s |", put)
}

func (c *Window) edCommandsForDir() string {
	return fmt.Sprintf(" Del Snarf Get |")
}

func (c *Window) edCommandsForErrorsWindow() string {
	return fmt.Sprintf(" Del Snarf |")
}

func (c *Window) customEdCommandsSet() bool {
	return c.customEdCommands != ""
}

func (c *Window) userArea(path string) (string, error) {
	var userArea string

	if c.initialTagUserArea != "" {
		userArea = c.initialTagUserArea
		if strings.HasSuffix(path, "+Errors") {
			userArea = " Clr" + userArea
		}

		c.initialTagUserArea = ""
	}

	if userArea == "" {
		_, _, userArea = mylog.Check4(c.Tag.Parts())
	}

	return userArea, nil
}

// markTextAsUnchanged marks the window body text to be the same as the
// contents on disk. This is used to decide whether to display the Put command.
func (w *Window) markTextAsUnchanged() {
	w.Body.text.Mark()
}

func (w *Window) LoadFile(path string) {
	w.LoadFileAndGoto(path, seek{}, selectText, growBodyIfTooSmall)
}

func (w *Window) LoadFileAndGoto(path string, goTo seek, selectBehaviour selectBehaviour, growBodyBehaviour growBodyBehaviour) {
	var ldr FileLoader
	w.Body.SetTextString("")
	w.markTextAsUnchanged()
	load := mylog.Check2(ldr.LoadAsync(path))
	wl := &WindowDataLoad{
		DataLoad:          *load,
		Win:               NewWindowHolder(w),
		Jobname:           filepath.Base(path),
		Goto:              goTo,
		SelectBehaviour:   selectBehaviour,
		GrowBodyBehaviour: growBodyBehaviour,
	}
	wl.Start(editor.WorkChan())
	editor.AddJob(wl)
	w.SetFilenameAndTag(path, typeUnknown)
	w.RemoveUndoHistoryFromTag()
}

func (w *Window) RemoveUndoHistoryFromTag() {
	w.Tag.SetTextStringNoUndo(w.Tag.String())
}

func (w *Window) Put() error {
	if w.file == "" {
		editor.AppendError("", "Can't Put: filename is empty")
		return fmt.Errorf("Can't Put with an empty filename")
	}

	var ldr FileLoader
	b := w.Body.Bytes()

	// err := ldr.Save(w.file, b)
	save := mylog.Check2(ldr.SaveAsync(w.file, b))

	ws := &WindowDataSave{
		Jobname: filepath.Base(w.file),
		Win:     w,
		errs:    save.Errs,
		kill:    save.Kill,
	}
	ws.Start(editor.WorkChan())
	editor.AddJob(ws)

	// w.markTextAsUnchanged()
	// w.SetTag()

	return nil
}

func (w *Window) Get() error {
	return w.GetWithSelect(dontSelectText, growBodyIfTooSmall)
}

func (w *Window) GetWithSelect(selectBehaviour selectBehaviour, growBodyBehaviour growBodyBehaviour) error {
	ci := w.Body.blockEditable.firstCursorIndex()

	w.LoadFileAndGoto(w.file, seek{seekType: seekToRunePos, runePos: ci}, selectBehaviour, growBodyBehaviour)

	w.Tag.clearSelections()

	return nil
}

type FillEditableWithItemList struct {
	items     []string
	render    *TextRenderer
	lastWidth int
}

func NewFillEditableWithItemList(l *layouter, style *Style, items []string) *FillEditableWithItemList {
	r := NewTextRenderer(l.curFont(), l.curFontSize(), style.LineSpacing, Color{})
	r.SetTabStopInterval(style.TabStopInterval)

	return &FillEditableWithItemList{
		items:  items,
		render: r,
	}
}

func (f *FillEditableWithItemList) AppendItems(items []string) {
	f.items = append(f.items, items...)
	f.lastWidth = 0 // Force a redraw
}

func (f *FillEditableWithItemList) preDrawHook(e *editable, gtx layout.Context) {
	w := gtx.Constraints.Max.X
	if w == f.lastWidth {
		return
	}

	b := f.render.LayoutItemsInColumns(gtx, f.items)
	// Add a few extra blank lines to make it easy to append commands to the end of the directory output.
	b = append(b, '\n')
	b = append(b, '\n')
	e.SetText(b)
	f.lastWidth = w
}

func (c *Window) SetContents(file string, typ fileType, b []byte) {
	c.SetFilenameAndTag(file, typ)
	c.Body.SetText(b)
}

func (c *Window) SetFilenameAndTag(file string, t fileType) {
	c.file = c.ensureDirEndsInSlash(file, t)
	c.setBodyCompletionSource()
	c.fileType = t
	c.SetTag()
}

func (c *Window) ensureDirEndsInSlash(file string, t fileType) string {
	if t != typeDir {
		return file
	}

	slash := string(filepath.Separator)
	p := mylog.Check2(NewGlobalPath(file, GlobalPathIsDir))

	if p.IsRemote() {
		slash = "/"
	}

	if !strings.HasSuffix(file, slash) {
		return file + slash
	}
	return file
}

func (c *Window) UpdateFilenameFromTag() {
	// We support filenames with spaces using the same heuristic as Russ Cox used for acme in plan9port:
	// See https://github.com/rsc/plan9port/commit/6267213474dd5449c161ca2e68ee16d9c0ffba07
	/*  " |" ends left half of tag
	 * If we find " Del Snarf" in the left half of the tag
	 * (before the pipe), that ends the file name.
	 */
	tag := string(c.Tag.Bytes())
	n := strings.Index(tag, " |")
	if n < 0 {
		return
	}

	n = strings.Index(tag[:n], " Del Snarf")
	if n < 0 {
		return
	}

	c.file = tag[:n]
	c.setBodyCompletionSource()
}

func (c *Window) Append(b []byte) {
	c.Body.Append(b)
}

func (c *Window) Zerox() (nw *Window, err error) {
	if c.fileType == typeDir {
		mylog.Check(fmt.Errorf("not allowed on directories\n"))
		return
	}

	nw = editor.NewWindow(nil)
	if nw == nil {
		mylog.Check(fmt.Errorf("failed to create window\n"))
		return
	}

	// The body of the new window and the current window will share the same piece table
	nw.Body.text = c.Body.text

	nw.SetFilenameAndTag(c.file, c.fileType)

	c.addClone(nw)
	nw.addClone(c)

	nw.Body.blockEditable.CursorIndices = make([]int, len(c.Body.blockEditable.CursorIndices))
	copy(nw.Body.blockEditable.CursorIndices, c.Body.blockEditable.CursorIndices)
	nw.Body.blockEditable.TopLeftIndex = c.Body.blockEditable.TopLeftIndex

	nw.maybeEnableSyntax()
	return
}

func (c *Window) BodyHeight() int {
	return c.bodyDims.Size.Y
}

func (w *Window) GrowIfBodyTooSmall() {
	if w.BodyHeight() < w.layout.lineHeight*9 && w.col != nil {
		w.col.Grow(w)
	}
}

func (w *Window) addClone(c *Window) {
	if w.clones == nil {
		w.clones = make(map[*Window]struct{})
	}

	w.clones[c] = struct{}{}
}

func (w *Window) removeClone(c *Window) {
	if w.clones == nil {
		return
	}

	delete(w.clones, c)
}

func (w *Window) hasClones() bool {
	return len(w.clones) > 0
}

func (w *Window) redrawClonesOnTextChange(ch *TextChange) {
	for c := range w.clones {
		if c == w {
			continue
		}

		// Don't notify us.
		c.Body.textChanged(dontFireListeners, *ch)

		c.Body.AddOpForNextLayout(func(gtx layout.Context) {
			if ch.Length != 0 {
				log(LogCatgWin, "redrawClonesOnTextChange: changing top left index of editable from %d to %d\n", c.Body.TopLeftIndex, c.Body.TopLeftIndex+ch.Length)
				w.shiftClonesTopLeftDueToTextModification(&c.Body, ch)
				c.Body.shiftItemsDueToTextModification(ch.Offset, ch.Length)
			}
			// This is to force a redraw
			c.Body.invalidateLayedoutText()
		})
	}
}

func (w *Window) shiftClonesTopLeftDueToTextModification(cloneBody *Body, ch *TextChange) {
	if cloneBody.TopLeftIndex >= ch.Offset {
		cloneBody.TopLeftIndex += ch.Length
	}
}

func (w *Window) removeFromAllClones() {
	for c := range w.clones {
		if c == w {
			continue
		}

		c.removeClone(w)
	}
}

func (w *Window) maybeEnableSyntax() {
	if w.fileType == typeFile {
		w.Body.EnableSyntax(w.file)
		w.setBodyCompletionSource()
		w.Body.BuildCompletions()
		w.Body.HighlightSyntax()
	}
}

func (w *Window) IsErrorsWindow() bool {
	return strings.HasSuffix(w.file, "+Errors")
}

func (w *Window) CanDelete() bool {
	if w.IsErrorsWindow() || w.fileType == typeDir {
		return true
	}

	if w.hasClones() {
		// If there are clones, we're just closing a view of the window; there's still
		// one open.
		return true
	}

	if w.bodyChangedFromDisk() && !w.allowDirtyDelete {
		return false
	}
	return true
}

func (w *Window) SetAllowDirtyDelete(b bool) {
	w.allowDirtyDelete = b
}

func (w *Window) disallowDirtyDelete(c *TextChange) {
	w.SetAllowDirtyDelete(false)
}

func (w *Window) notifyApiBodyChanged(c *TextChange) {
	n := ApiNotification{
		WinId:  w.Id,
		Offset: c.Offset,
		Len:    c.Length,
	}

	if c.Length >= 0 {
		n.Op = ApiNotificationOpInsert
	} else {
		n.Op = ApiNotificationOpDelete
		n.Len = -n.Len
	}

	addApiNotificationToAllSessions(n)
}

func (w *Window) SetStyle(style Style) {
	w.layout.style = style
	w.layout.layouter.fontStyles = style.Fonts
	w.setLineHeight()
	w.Tag.SetStyle(style.tagBlockStyle(), style.tagEditableStyle())
	w.Body.SetStyle(style.bodyBlockStyle(), style.bodyEditableStyle(), style.Syntax)
	w.layoutBox.SetStyle(style.layoutBoxStyle(), w.layout.lineHeight)
	w.scrollbar.SetStyle(style.scrollbarStyle())
}

func (w *Window) showIfHidden() {
	max := w.col.MaximizedWindow()
	if max != nil && max != w {
		w.col.Optimize()
	}
}

func (w *Window) setFlash(b bool) {
	w.Tag.SetFlash(b)
}

func (w *Window) InterceptEvent(gtx layout.Context, ev event.Event) (processed bool) {
	// This is used to snoop events from the scrollbar and layoutbox in order to
	// mark windows as unflashed whenever a scrollbar or layoutbox is clicked.
	pe, ok := ev.(*pointer.Event)
	if !ok {
		return
	}

	if pe.Type != pointer.Press {
		return
	}

	editor.SetOnlyFlashedWindow(nil)
	return false
}

func (w *Window) centerBodyOnFirstCursorOrPrimarySelection() {
	w.Body.AddOpForNextLayout(func(gtx layout.Context) {
		w.Body.centerOnFirstCursorOrPrimarySelection(gtx)
	})
}

func (w *Window) setBodyCompletionSource() {
	src := w.file
	if w.file == "" {
		src = fmt.Sprintf("unnamed-%p", w.Body.Tag())
	}
	w.Body.completionSource = src
}

func (w *Window) greyoutIfOpIsTakingTooLong(opFinished chan struct{}) {
	if w == nil {
		return
	}

	tmr := time.NewTimer(2 * time.Second)

	greyout := func() {
		w.overlayWithGrey = true
	}

	unGreyout := func() {
		w.overlayWithGrey = false
	}

loop:
	for {
		select {
		case <-tmr.C:
			editor.WorkChan() <- basicWork{greyout}
		case <-opFinished:
			editor.WorkChan() <- basicWork{unGreyout}
			break loop
		}
	}
}
