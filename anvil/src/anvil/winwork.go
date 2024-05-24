package main

import (
	"gioui.org/layout"
	"github.com/ddkwork/golibrary/mylog"
)

type WindowDataLoad struct {
	DataLoad
	Jobname string
	Win     WindowHolder
	// Win        *Window
	// ErrWinName string
	Goto              seek
	Tail              bool
	SelectBehaviour   selectBehaviour
	GrowBodyBehaviour growBodyBehaviour
	Job               Job
}

type WindowHolder struct {
	win     *Window
	winName string
}

func (h *WindowHolder) Get() *Window {
	if h.win != nil {
		return h.win
	}
	h.win = editor.FindOrCreateWindow(h.winName)
	return h.win
}

func (h *WindowHolder) LoadByName() bool {
	return h.winName != ""
}

func NewWindowHolder(win *Window) WindowHolder {
	return WindowHolder{win: win}
}

func NewWindowHolderForName(winName string) WindowHolder {
	return WindowHolder{winName: winName}
}

func (f *WindowDataLoad) Start(c chan Work) {
	go f.pump(c)
}

func (f *WindowDataLoad) GetJob() Job {
	if f.Job == nil {
		return f
	}
	return f.Job
}

type WindowDataLoadSender struct {
	contentsClosed  bool
	filenamesClosed bool
	errsClosed      bool
	sentType        bool
	work            chan Work
	load            *WindowDataLoad
}

func (w WindowDataLoadSender) workIsDone() bool {
	return (w.contentsClosed && w.errsClosed) || (w.filenamesClosed && w.errsClosed)
}

func (w *WindowDataLoadSender) sendType(t fileType) {
	if w.sentType {
		return
	}

	w.work <- &winSetFiletype{job: w.load.GetJob(), win: w.load.Win.Get(), fileType: t}
	w.sentType = true
}

func (w *WindowDataLoadSender) updateStateWhenContentsClosed() {
	log(LogCatgWin, "pump: contents is closed\n")
	w.contentsClosed = true
	w.load.Contents = nil
}

func (w *WindowDataLoadSender) sendContents(x []byte) {
	w.sendType(typeFile)

	log(LogCatgWin, "pump: got some contents\n")
	w.work <- &winLoadData{job: w.load.GetJob(), win: w.load.Win.Get(), data: x, growBodyBehaviour: w.load.GrowBodyBehaviour}
	if w.load.Tail {
		w.work <- &winLoadGoToEnd{job: w.load.GetJob(), win: w.load.Win.Get()}
	}
}

func (w *WindowDataLoadSender) updateStateWhenFilenamesClosed() {
	log(LogCatgWin, "pump: contents is closed\n")
	w.filenamesClosed = true
	w.load.Filenames = nil
}

func (w *WindowDataLoadSender) sendFilenames(x []string) {
	w.sendType(typeDir)
	w.work <- &winLoadNames{job: w.load.GetJob(), win: w.load.Win.Get(), names: x}
	log(LogCatgWin, "pump: got some filenames\n")
}

func (w *WindowDataLoadSender) updateStateWhenErrorsClosed() {
	log(LogCatgWin, "pump: errors is closed\n")
	w.errsClosed = true
	w.load.Errs = nil
}

func (w *WindowDataLoadSender) sendError(x error) {
	log(LogCatgWin, "pump: got an error\n")
	w.work <- &winLoadErr{job: w.load.GetJob(), win: w.load.Win.Get(), err: x}
}

func (w *WindowDataLoadSender) finalize() {
	// If we are writing this to an existing errors window, don't do any of the normal finalization actions,
	// just signify that the job is complete. This is to prevent popping up an empty errors window
	if w.load.Win.LoadByName() {
		w.work <- &winLoadDone{job: w.load.GetJob(), selectBehaviour: w.load.SelectBehaviour}
		return
	}

	// In case there is no such file (like in the case of a New for a non-existent file),
	// set the window to be a file
	w.sendType(typeFile)

	log(LogCatgWin, "pump done\n")
	w.work <- &winLoadDone{job: w.load.GetJob(), win: w.load.Win.Get(), goTo: w.load.Goto, selectBehaviour: w.load.SelectBehaviour}
	close(w.load.DataLoad.Kill)
}

func (f *WindowDataLoad) pump(c chan Work) {
	log(LogCatgWin, "pump started\n")

	sender := WindowDataLoadSender{
		work: c,
		load: f,
	}

FOR:
	for {
		select {
		case x, ok := <-f.Contents:
			if !ok {
				sender.updateStateWhenContentsClosed()
				if sender.workIsDone() {
					break FOR
				}
				break
			}

			sender.sendContents(x)
		case x, ok := <-f.Filenames:
			if !ok {
				sender.updateStateWhenFilenamesClosed()
				if sender.workIsDone() {
					break FOR
				}
				break
			}

			sender.sendFilenames(x)
		case x, ok := <-f.Errs:
			if !ok {
				sender.updateStateWhenErrorsClosed()
				if sender.workIsDone() {
					break FOR
				}
				break
			}

			sender.sendError(x)
		}
	}

	sender.finalize()
	log(LogCatgWin, "pump finished\n")
}

type growBodyBehaviour int

const (
	growBodyIfTooSmall = iota
	dontGrowBodyIfTooSmall
)

func (l *WindowDataLoad) Kill() {
	select {
	case l.DataLoad.Kill <- struct{}{}:
	default:
	}
}

func (l *WindowDataLoad) Name() string {
	return l.Jobname
}

// WindowDataChunk is a chunk of data to be written to a window, or an error
type winLoadData struct {
	job               Job
	win               *Window
	data              []byte
	growBodyBehaviour growBodyBehaviour
}

type winLoadNames struct {
	job   Job
	win   *Window
	names []string
}

type winLoadErr struct {
	job Job
	win *Window
	err error
}

type winLoadDone struct {
	job             Job
	win             *Window
	goTo            seek
	selectBehaviour selectBehaviour
}

type winLoadGoToEnd struct {
	job Job
	win *Window
}

type winSetFiletype struct {
	job      Job
	win      *Window
	fileType fileType
}

type Work interface {
	Service() (done bool)
	// Which job is it for
	Job() Job
}

func (l winLoadData) Service() (done bool) {
	l.win.Append(l.data)
	if l.growBodyBehaviour == growBodyIfTooSmall {
		l.win.showIfHidden()
		l.win.GrowIfBodyTooSmall()
		editor.SetOnlyFlashedWindow(l.win)
	}

	log(LogCatgWin, "Appended %d bytes to window %s\n", len(l.data), l.win.file)
	return false
}

func (l winLoadData) Job() Job {
	return l.job
}

func (l winLoadNames) Service() (done bool) {
	l.win.filler.AppendItems(l.names)
	l.win.SetTag()
	return false
}

func (l winLoadNames) Job() Job {
	return l.job
}

func (l winLoadErr) Service() (done bool) {
	dir := ""
	if l.win != nil {
		f := NewFileFinder(l.win)
		d := mylog.Check2(f.WindowDir())
		dir = d
	}
	editor.AppendError(dir, l.err.Error())
	return true
}

func (l winLoadErr) Job() Job {
	return l.job
}

func (l winLoadDone) Service() (done bool) {
	if l.win != nil {
		l.win.markTextAsUnchanged()
		l.win.SetTag()
		l.win.Body.AddOpForNextLayout(func(gtx layout.Context) {
			// This is to force a redraw
			l.win.Body.invalidateLayedoutText()
		})
		if !l.goTo.empty() {
			log(LogCatgWin, "winLoadDone: adding a goto for next layout. Goto: %v\n", l.goTo)
			l.win.Body.AddOpForNextLayout(func(gtx layout.Context) {
				log(LogCatgWin, "editable next layout op called: goto %v\n", l.goTo)
				l.win.Body.moveCursorTo(gtx, l.goTo, l.selectBehaviour)
			})
		}
		l.win.maybeEnableSyntax()
	}
	return true
}

func (l winLoadDone) Job() Job {
	return l.job
}

func (l winLoadGoToEnd) Service() (done bool) {
	l.win.Body.AddOpForNextLayout(func(gtx layout.Context) {
		l.win.Body.moveToEndOfDoc(gtx)
	})
	return false
}

func (l winLoadGoToEnd) Job() Job {
	return l.job
}

func (l winSetFiletype) Service() (done bool) {
	l.win.SetFilenameAndTag(l.win.file, l.fileType)

	if l.fileType == typeDir {
		l.win.filler = NewFillEditableWithItemList(&l.win.Body.layouter, &l.win.layout.style, []string{})
		l.win.Body.SetPreDrawHook(l.win.filler.preDrawHook)
	} else {
		l.win.Body.SetPreDrawHook(nil)
	}

	return false
}

func (l winSetFiletype) Job() Job {
	return l.job
}

type WindowDataSave struct {
	Jobname string
	Win     *Window
	errs    chan error
	kill    chan struct{}
}

func (s WindowDataSave) Name() string {
	return s.Jobname
}

func (s WindowDataSave) Kill() {
	select {
	case s.kill <- struct{}{}:
	default:
	}
}

func (s *WindowDataSave) Start(c chan Work) {
	go s.wait(c)
}

func (s *WindowDataSave) wait(c chan Work) {
	defer close(s.kill)

	e, ok := <-s.errs
	if !ok {
		// errors closed
		c <- &winSaveDone{job: s, win: s.Win}
		return
	}
	c <- &winLoadErr{job: s, err: e}
}

type winSaveDone struct {
	job Job
	win *Window
}

func (l winSaveDone) Service() (done bool) {
	l.win.markTextAsUnchanged()
	l.win.SetTag()
	return true
}

func (l winSaveDone) Job() Job {
	return l.job
}
