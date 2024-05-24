package main

import (
	"fmt"

	"gioui.org/layout"
	"github.com/ddkwork/golibrary/mylog"
)

// adapter is the interface between the editable and the environment it is
// embedded in (the editor)
type adapter interface {
	completeFilename(word string, callback CompletionsCallback)
	appendError(dir, msg string)
	copyAllSelectionsFromLastSelectedEditable(gtx layout.Context)
	cutAllSelectionsFromLastSelectedEditable(gtx layout.Context)
	textOfAllSelectionsInLastSelectedEditable() []string
	pasteToFocusedEditable(gtx layout.Context)
	execute(e *editable, gtx layout.Context, cmd string, args []string)
	plumb(e *editable, gtx layout.Context, obj string) (plumbed bool)
	loadFileAndGoto(gtx layout.Context, path string, opts LoadFileOpts)
	loadFile(gtx layout.Context, path string)
	loadFileInPlaceAndGoto(gtx layout.Context, path string, opts LoadFileOpts)
	loadFileInPlace(gtx layout.Context, path string)
	textOfLastSelectionInEditor() string
	shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange int)
	setFocusedEditable(e *editable)
	focusedEditable() *editable
	findFile(file string) (path *GlobalPath, err error)
	dir() string
	put()
	get()
	file() string
	mark(markName, file string, cursorIndex int)
	gotoMark(markName string)
	doWork(w Work)
	replaceCrWithTofu() bool
	setShellString(s string)
}

// editableAdapter connects an editable with the rest of the editor (it's owning window, etc)
// so that it has less dependencies
type editableAdapter struct {
	fileFinder *FileFinder
	executor   *CommandExecutor
	// owner is the owner of the editable: a Window, Col or Editor.
	owner       interface{}
	shellString string
}

func (a editableAdapter) completeFilename(word string, callback CompletionsCallback) {
	dir, base := mylog.Check3(computeDirAndBaseForFilenameCompletion(word, a.fileFinder))
	log(LogCatgCompletion, "adapter: Complete on dir='%s' base='%s'\n", dir, base)
	mylog.

		// This will call editable.applyFilenameCompletions when complete
		Check(FilenameCompletionsAsync(word, dir, base, callback))
}

func (a editableAdapter) appendError(dir, msg string) {
	editor.AppendError(dir, msg)
}

func (a editableAdapter) copyAllSelectionsFromLastSelectedEditable(gtx layout.Context) {
	editor.copyAllSelectionsFromLastSelectedEditable(gtx)
}

func (a editableAdapter) cutAllSelectionsFromLastSelectedEditable(gtx layout.Context) {
	editor.cutAllSelectionsFromLastSelectedEditable(gtx)
}

func (a editableAdapter) pasteToFocusedEditable(gtx layout.Context) {
	editor.pasteToFocusedEditable(gtx)
}

func (a editableAdapter) execute(e *editable, gtx layout.Context, cmd string, args []string) {
	if args == nil {
		args = []string{}
	}

	log(LogCatgCompletion, "adapter: Execute %s %v\n", cmd, args)
	if a.executor != nil {
		ctx := a.buildCmdContext(e, gtx, args)
		a.executor.Do(cmd, ctx)
	}
}

func (a editableAdapter) dir() string {
	dir := mylog.Check2(a.fileFinder.WindowDir())

	return dir
}

func (a editableAdapter) findFile(file string) (path *GlobalPath, err error) {
	_, err2 := a.fileFinder.WindowDir()
	if err2 != nil {
		_ = ""
	}

	path, _ = mylog.Check3(a.fileFinder.Find(file))

	return
}

func (a editableAdapter) buildCmdContext(e *editable, gtx layout.Context, args []string) *CmdContext {
	dir := mylog.Check2(a.fileFinder.WindowDir())

	file := mylog.Check2(a.fileFinder.WindowFile())

	return &CmdContext{
		Gtx:         gtx,
		Dir:         dir,
		Editable:    e.executeOn,
		Args:        args,
		Path:        file,
		Selections:  e.selections,
		ShellString: a.shellString,
	}
}

func (a *editableAdapter) setShellString(s string) {
	a.shellString = s
}

func (a editableAdapter) plumb(e *editable, gtx layout.Context, obj string) (plumbed bool) {
	if plumber != nil && a.executor != nil {
		ctx := a.buildCmdContext(e, gtx, nil)

		plumbed = mylog.Check2(plumber.Plumb(obj, a.executor, ctx))

	}
	return
}

func (a editableAdapter) column() *Col {
	var col *Col

	switch v := a.owner.(type) {
	case Window:
	case *Window:
		col = v.col
	case Col:
		col = &v
	case *Col:
		col = v
	}

	return col
}

func (a editableAdapter) loadFileAndGoto(gtx layout.Context, path string, opts LoadFileOpts) {
	opts.InCol = a.column()
	w := editor.LoadFileOpts(path, opts)
	if w != nil {
		w.SetFocus(gtx)
	}
}

func (a editableAdapter) loadFile(gtx layout.Context, path string) {
	var opts LoadFileOpts
	opts.InCol = a.column()
	w := editor.LoadFileOpts(path, opts)
	if w != nil {
		w.SetFocus(gtx)
	}
}

func (a editableAdapter) loadFileInPlaceAndGoto(gtx layout.Context, path string, opts LoadFileOpts) {
	win, ok := a.owner.(*Window)
	if !ok {
		return
	}

	win.LoadFileAndGoto(path, opts.GoTo, opts.SelectBehaviour, opts.GrowBodyBehaviour)
}

func (a editableAdapter) loadFileInPlace(gtx layout.Context, path string) {
	win, ok := a.owner.(*Window)
	if !ok {
		return
	}

	win.LoadFile(path)
}

func (a editableAdapter) textOfLastSelectionInEditor() string {
	sel := editor.lastSelection
	if sel.isSet && sel.editable != nil {
		return sel.editable.textOfSelection(sel.sel)
	}
	return ""
}

func (a editableAdapter) textOfAllSelectionsInLastSelectedEditable() []string {
	sel := editor.lastSelection
	ed := sel.editable
	if !sel.isSet || ed == nil {
		return nil
	}

	res := []string{}
	for _, s := range ed.selections {
		res = append(res, ed.textOfSelection(s))
	}
	return res
}

func (a editableAdapter) shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange int) {
	file := mylog.Check2(a.fileFinder.WindowFile())
	editor.Marks.ShiftDueToTextModification(file, startOfChange, lengthOfChange)
}

func (a editableAdapter) setFocusedEditable(e *editable) {
	w := (*Window)(nil)
	if win, ok := a.owner.(*Window); ok {
		w = win
	}

	editor.setFocusedEditable(e, w)
}

func (a editableAdapter) focusedEditable() *editable {
	return editor.getFocusedEditable()
}

func (a editableAdapter) put() {
	w, ok := a.owner.(*Window)
	if ok {
		w.Put()
	}
}

func (a editableAdapter) get() {
	w, ok := a.owner.(*Window)
	if ok {
		w.Get()
	}
}

func (a editableAdapter) file() string {
	file := ""
	w, ok := a.owner.(*Window)
	if ok {
		file = w.file
	}
	return file
}

func (a editableAdapter) mark(markName, file string, cursorIndex int) {
	editor.Marks.Set(markName, file, cursorIndex)
}

func (a editableAdapter) gotoMark(markName string) {
	file, seek, ok := editor.Marks.Seek(markName)
	if ok {
		editor.LoadFileOpts(file, LoadFileOpts{GoTo: seek, SelectBehaviour: dontSelectText})
	}
}

func (a editableAdapter) doWork(w Work) {
	editor.WorkChan() <- w
}

func (a editableAdapter) replaceCrWithTofu() bool {
	return settings.Typesetting.ReplaceCRWithTofu
}

type nilAdapter struct{}

func (a nilAdapter) completeFilename(word string, callback CompletionsCallback)         {}
func (a nilAdapter) appendError(dir, msg string)                                        {}
func (a nilAdapter) copyAllSelectionsFromLastSelectedEditable(gtx layout.Context)       {}
func (a nilAdapter) cutAllSelectionsFromLastSelectedEditable(gtx layout.Context)        {}
func (a nilAdapter) textOfAllSelectionsInLastSelectedEditable() []string                { return nil }
func (a nilAdapter) pasteToFocusedEditable(gtx layout.Context)                          {}
func (a nilAdapter) execute(e *editable, gtx layout.Context, cmd string, args []string) {}
func (a nilAdapter) plumb(e *editable, gtx layout.Context, obj string) (plumbed bool)   { return false }
func (a nilAdapter) loadFileAndGoto(gtx layout.Context, path string, opts LoadFileOpts) {
}
func (a nilAdapter) loadFile(gtx layout.Context, path string)                                {}
func (a nilAdapter) textOfLastSelectionInEditor() string                                     { return "" }
func (a nilAdapter) shiftEditorItemsDueToTextModification(startOfChange, lengthOfChange int) {}
func (a nilAdapter) setFocusedEditable(e *editable)                                          {}
func (a nilAdapter) focusedEditable() *editable                                              { return nil }
func (a nilAdapter) findFile(file string) (path *GlobalPath, err error) {
	return nil, fmt.Errorf("not implemented")
}
func (a nilAdapter) dir() string                                                               { return "" }
func (a nilAdapter) put()                                                                      {}
func (a nilAdapter) get()                                                                      {}
func (a nilAdapter) file() string                                                              { return "" }
func (a nilAdapter) mark(markName, file string, cursorIndex int)                               {}
func (a nilAdapter) gotoMark(markName string)                                                  {}
func (a nilAdapter) doWork(w Work)                                                             {}
func (a nilAdapter) loadFileInPlaceAndGoto(gtx layout.Context, path string, opts LoadFileOpts) {}
func (a nilAdapter) loadFileInPlace(gtx layout.Context, path string)                           {}
func (a nilAdapter) replaceCrWithTofu() bool                                                   { return false }
func (a nilAdapter) setShellString(s string)                                                   {}
