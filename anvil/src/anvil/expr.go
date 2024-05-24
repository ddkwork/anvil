package main

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/ddkwork/golibrary/mylog"
	"github.com/jeffwilliams/anvil/internal/expr"
	"github.com/jeffwilliams/anvil/internal/pctbl"
	"github.com/jeffwilliams/anvil/internal/runes"
)

type ExprHandler struct {
	pieceTable pctbl.Table
	// Call this after one of the changes below occurs
	afterChanged func()
	file         string
	dir          string
	data         []byte
	editable     *editable
	toDisplay    bytes.Buffer
	cursorIndex  int
}

func (handler ExprHandler) Delete(r expr.Range) {
	l := r.End() - r.Start()
	log(LogCatgExpr, "editable expr handler: performing delete of length %d at %d", l, r.Start())
	handler.sendWork(func() {
		handler.editable.deleteFromPieceTableUndoIndex(r.Start(), l, handler.cursorIndex)
	})
}

func (handler ExprHandler) Insert(index int, value []byte) {
	log(LogCatgExpr, "editable expr handler: performing insert of '%s' at %d", string(value), index)
	l := utf8.RuneCount(value)
	handler.sendWork(func() {
		handler.editable.insertToPieceTableUndoIndex(index, string(value), handler.cursorIndex)
		s := selection{index, index + l}
		handler.selectRange(s)
	})
}

func (handler *ExprHandler) Display(r expr.Range) {
	w := runes.NewWalker(handler.data)
	sline := 1
	var scol int
	i := 0
	lastr := ' '
	for ; i < r.Start(); i++ {
		w.Forward(1)
		if lastr == '\n' {
			sline++
			scol = 0
		}
		scol++
		lastr = w.Rune()
	}

	fline := sline
	var fcol int
	for ; i < r.End(); i++ {
		w.Forward(1)
		if lastr == '\n' {
			fline++
			fcol = 0
		}
		fcol++
		lastr = w.Rune()
	}

	if sline == fline {
		fmt.Fprintf(&handler.toDisplay, "%s:%d ", handler.file, sline)
		if scol != fcol {
			fmt.Fprintf(&handler.toDisplay, "( %s:%d:%d )", handler.file, sline, scol)
		}
	} else {
		fmt.Fprintf(&handler.toDisplay, "%s:%d – %s:%d ", handler.file, sline, handler.file, fline)
		fmt.Fprintf(&handler.toDisplay, "( %s:%d:%d – %s:%d:%d )", handler.file, sline, scol, handler.file, fline, fcol)
	}
	handler.toDisplay.WriteRune('\n')
}

func (handler *ExprHandler) DisplayContents(r expr.Range, prefix string) {
	w := runes.NewWalker(handler.data)
	b := w.TextBetweenRuneIndices(r.Start(), r.End())
	handler.toDisplay.WriteString(prefix)
	handler.toDisplay.Write(b)

	handler.sendWork(func() {
		handler.selectRange(r)
	})
}

func (handler ExprHandler) Noop(r expr.Range) {
	handler.sendWork(func() {
		handler.selectRange(r)
	})
}

func (handler ExprHandler) selectRange(r expr.Range) {
	handler.editable.AddSelection(r.Start(), r.End())
}

func (handler ExprHandler) Done() {
	handler.sendWork(handler.done)
}

func (handler ExprHandler) done() {
	if handler.toDisplay.Len() > 0 {
		editor.AppendError(handler.dir, handler.toDisplay.String())
	}

	if handler.afterChanged != nil {
		handler.afterChanged()
	}
}

func (handler ExprHandler) sendWork(f func()) {
	editor.WorkChan() <- exprHandlerWork{handler.editable, f}
}

type EditableExprExecutor struct {
	editable *editable
	handler  *ExprHandler
	dir      string
	vm       expr.Interpreter
	win      *Window
}

func NewEditableExprExecutor(e *editable, win *Window, dir string, handler *ExprHandler) EditableExprExecutor {
	return EditableExprExecutor{
		editable: e,
		handler:  handler,
		dir:      dir,
		win:      win,
	}
}

func (ex EditableExprExecutor) Do(cmd string) {
	ok := ex.createInterpreter(cmd)
	if !ok {
		return
	}

	ranges := ex.buildInitialRanges()
	ex.log(cmd, ranges)
	// ex.runInterpreter(ranges)
	ex.runInterpreterAsync(ranges)
}

func (ex *EditableExprExecutor) createInterpreter(cmd string) (ok bool) {
	var s expr.Scanner
	toks, ok := s.Scan(cmd)
	if !ok {
		editor.AppendError(ex.dir, "Scanning addressing expression failed")
		return
	}

	var p expr.Parser
	p.SetMatchLimit(1000)
	tree := mylog.Check2(p.Parse(toks))

	ex.vm = mylog.Check2(expr.NewInterpreter(ex.handler.data, tree, ex.handler, ex.editable.firstCursorIndex()))

	return true
}

func (ex *EditableExprExecutor) buildInitialRanges() []expr.Range {
	ranges := make([]expr.Range, len(ex.editable.selections))
	for i, sel := range ex.editable.selections {
		ranges[i] = sel
	}
	if len(ranges) == 0 {
		ranges = append(ranges, selection{0, utf8.RuneCount(ex.handler.data)})
	}

	return ranges
}

func (ex *EditableExprExecutor) log(cmd string, ranges []expr.Range) {
	log(LogCatgCmd, "Executing addressing expression %s on ranges ", cmd)
	for _, r := range ranges {
		log(LogCatgCmd, "(%d,%d) ", r.Start(), r.End())
	}
	fmt.Println()
}

func (ex *EditableExprExecutor) runInterpreter(initialRanges []expr.Range) {
	ex.editable.StartTransaction()
	mylog.Check(ex.vm.Execute(initialRanges))
	ex.editable.EndTransaction()
}

func (ex *EditableExprExecutor) runInterpreterAsync(initialRanges []expr.Range) {
	ex.editable.StartTransaction()
	ex.editable.writeLock.lock()
	// The code that saves deletes in OptimizedPieceTable is slow and we don't need
	// it when doing expressions.
	ex.editable.SetSaveDeletes(false)

	finished := make(chan struct{})
	go ex.win.greyoutIfOpIsTakingTooLong(finished)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dumpPanic(r)
				dumpLogs()
				dumpGoroutines()
				panic(r)
			}
		}()
		mylog.Check(ex.vm.Execute(initialRanges))
		editor.WorkChan() <- basicWork{func() {
			ex.editable.writeLock.unlock()
			ex.editable.SetSaveDeletes(true)
		}}
		ex.editable.EndTransaction()
		finished <- struct{}{}
	}()
}

type exprHandlerWork struct {
	editable *editable
	f        func()
}

func (w exprHandlerWork) Service() (done bool) {
	w.editable.writeLock.unlock()
	w.f()
	w.editable.writeLock.lock()
	return true
}

func (w exprHandlerWork) Job() Job {
	return nil
}
