package main

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"time"

	"gioui.org/app"
	"github.com/ddkwork/golibrary/mylog"
)

type ApplicationState struct {
	Title           string
	Editor          *EditorState
	AppWindowCfgSet bool
	AppWindowSize   image.Point
	AppWindowMode   app.WindowMode
	WinIdGenState   *IdGenState
	CommandHistory  *CommandHistoryState
}

func (a *Application) State() *ApplicationState {
	s := &ApplicationState{
		Title:          application.appWindowTitle,
		Editor:         editor.State(),
		WinIdGenState:  a.winIdGenerator.State(),
		CommandHistory: cmdHistory.State(),
	}

	if a.winConfig != nil {
		s.AppWindowCfgSet = true
		s.AppWindowSize = a.winConfig.Size
		s.AppWindowMode = a.winConfig.Mode
	}

	return s
}

func (a *Application) SetState(state *ApplicationState) error {
	if state == nil {
		return fmt.Errorf("The application state is nil")
	}
	a.SetTitle(state.Title)
	editor.SetState(state.Editor)

	if state.AppWindowCfgSet {
		if state.AppWindowMode == app.Windowed {
			a.SetWindowSize(state.AppWindowSize)
		}
	}

	a.winIdGenerator.SetState(state.WinIdGenState)
	cmdHistory.SetState(state.CommandHistory)

	return nil
}

type EditorState struct {
	Tag         *TagState
	Cols        []*ColState
	RecentFiles []string
	Marks       MarkState
}

func (e *Editor) State() *EditorState {
	edTag := e.Tag.State()

	var cols []*ColState
	for _, c := range e.Cols {
		cols = append(cols, c.State())
	}

	// Remove any running jobs, since they won't be running after load.
	edTag.Text = e.removeJobsFromTag(edTag.Text)

	return &EditorState{
		Tag:         edTag,
		Cols:        cols,
		RecentFiles: editor.recentFiles.All(),
		Marks:       editor.Marks.State(),
	}

	// e.focusedEditable
}

func (e *Editor) removeJobsFromTag(tag string) string {
	for _, j := range e.jobs {
		tag, _, _ = removeJobFromTagString(j.Name(), tag)
	}
	return tag
}

func (e *Editor) SetState(state *EditorState) error {
	if state == nil {
		return fmt.Errorf("The editor state is nil")
	}
	e.Tag.SetState(state.Tag)

	// Remove all columns
	editor.Clear()

	anyVisible := false
	for _, c := range state.Cols {
		col := editor.NewColDontPosition()
		col.SetState(c)
		if col.Visible() {
			anyVisible = true
		}
	}
	if !anyVisible && len(editor.Cols) > 0 {
		editor.Cols[0].SetVisible(true)
		editor.ensureFirstVisibleColIsLeftJustified()
	}

	for _, f := range state.RecentFiles {
		editor.AddRecentFile(f)
	}

	editor.Marks.SetState(state.Marks)

	return nil
}

type TagState struct {
	Text string
}

func (t *Tag) State() *TagState {
	return &TagState{Text: t.String()}
}

func (t *Tag) SetState(s *TagState) error {
	if s == nil {
		return fmt.Errorf("The tag state is nil")
	}
	t.SetTextStringNoUndo(s.Text)
	return nil
}

type ColState struct {
	Tag     *TagState
	LeftX   int
	Windows []*WindowState
	Visible bool
}

func (c *Col) State() *ColState {
	var wins []*WindowState
	for _, w := range c.Windows {
		wins = append(wins, w.State())
	}

	return &ColState{
		Tag:     c.Tag.State(),
		LeftX:   c.LeftX,
		Windows: wins,
		Visible: c.visible,
	}
}

func (c *Col) SetState(state *ColState) error {
	if state == nil {
		return fmt.Errorf("The column state is nil")
	}
	c.Tag.SetState(state.Tag)
	c.LeftX = state.LeftX
	c.visible = state.Visible

	for _, w := range state.Windows {
		win := c.NewWindowDontPosition()
		win.SetState(w)
	}
	return nil
}

type WindowState struct {
	Tag                *TagState
	TopY               int
	Body               *BodyState
	File               string
	FileType           fileType
	Id                 int
	CloneIds           []int
	ManualHighlighting []ManualHighlightingInterval
}

type ManualHighlightingInterval struct {
	Start, End int
	Color      Color
}

func (w *Window) State() *WindowState {
	cloneIds := make([]int, len(w.clones))
	i := 0
	for c := range w.clones {
		cloneIds[i] = c.Id
		i++
	}

	attemptSavingContents := true
	if w.fileType == typeDir {
		attemptSavingContents = false
	}

	manualHighlighting := make([]ManualHighlightingInterval, len(w.Body.manualHighlighting))
	for i, v := range w.Body.manualHighlighting {
		manualHighlighting[i].Start = v.start
		manualHighlighting[i].End = v.end
		manualHighlighting[i].Color = v.color
	}

	return &WindowState{
		Tag:                w.Tag.State(),
		TopY:               w.TopY,
		Body:               w.Body.State(attemptSavingContents),
		File:               w.file,
		FileType:           w.fileType,
		Id:                 w.Id,
		CloneIds:           cloneIds,
		ManualHighlighting: manualHighlighting,
	}
}

func (w *Window) SetState(state *WindowState) error {
	if state == nil {
		return fmt.Errorf("The window state is nil")
	}
	w.Tag.SetState(state.Tag)
	w.TopY = state.TopY
	w.initialTagUserArea = ""
	w.SetFilenameAndTag(state.File, state.FileType)
	w.Body.SetState(state.Body)
	if state.Body.Text == "" {
		w.GetWithSelect(dontSelectText, dontGrowBodyIfTooSmall)
	}

	w.Body.manualHighlighting = make([]*SyntaxInterval, len(state.ManualHighlighting))
	for i, v := range state.ManualHighlighting {
		w.Body.manualHighlighting[i] = NewSyntaxInterval(v.Start, v.End, v.Color)
	}

	application.winIdGenerator.Free(w.Id)
	w.Id = state.Id

	// The clone we are searching for may not have been loaded yet.
	// But as we load more windows from the state dump we will eventually
	// load all missing windows, and they will get linked bidirectionally.
	for _, id := range state.CloneIds {
		clone := editor.FindWindowForId(id)
		if clone != nil {
			w.addClone(clone)
			clone.addClone(w)
			w.Body.text = clone.Body.text
		}
	}

	return nil
}

type BodyState struct {
	CursorIndices []int
	TopLeftIndex  int
	Text          string
	FontIndex     int
}

const MaxWindowBodyLenToDump = 4096

func (b *Body) State(attemptSavingContents bool) *BodyState {
	state := &BodyState{
		CursorIndices: b.CursorIndices,
		TopLeftIndex:  b.TopLeftIndex,
		FontIndex:     b.curFontIndex,
	}

	if attemptSavingContents {
		if !b.text.IsMarked() {
			// Not saved
			str := b.String()
			if len(str) < MaxWindowBodyLenToDump {
				state.Text = str
			}
		}
	}

	return state
}

func (b *Body) SetState(state *BodyState) error {
	if state == nil {
		return fmt.Errorf("The body state is nil")
	}
	if state.Text != "" {
		b.SetTextString(state.Text)
	}
	b.CursorIndices = state.CursorIndices
	b.TopLeftIndex = state.TopLeftIndex
	b.curFontIndex = state.FontIndex
	b.invalidateLayedoutText()
	b.initTextRenderer()
	return nil
}

type IdGenState struct {
	Free []int
	Next int
}

func (g *IdGen) State() *IdGenState {
	return &IdGenState{
		Next: g.next,
		Free: g.free,
	}
}

func (g *IdGen) SetState(state *IdGenState) error {
	if state == nil {
		return fmt.Errorf("The id generator state is nil")
	}
	g.next = state.Next
	g.free = state.Free
	return nil
}

func WriteState(path string, state interface{}) error {
	file := mylog.Check2(os.Create(path))

	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(state)
}

func ReadState(path string, state interface{}) error {
	file := mylog.Check2(os.Open(path))

	defer file.Close()

	enc := json.NewDecoder(file)
	return enc.Decode(state)
}

type CommandHistoryState struct {
	Cmds []CommandHistoryEntryState
}

type CommandHistoryEntryState struct {
	Cmd     string
	Started time.Time
	Ended   time.Time
	State   RunState
	Dir     string
}

func (c CommandHistory) State() *CommandHistoryState {
	state := &CommandHistoryState{
		Cmds: []CommandHistoryEntryState{},
	}

	c.cmds.Each(func(v *CommandHistoryEntry) {
		log(LogCatgApp, "CommandHistory.State: found a cmd entry\n")
		st := CommandHistoryEntryState{
			Cmd:     v.cmd,
			Started: v.started,
			Ended:   v.ended,
			State:   v.state,
			Dir:     v.dir,
		}

		state.Cmds = append(state.Cmds, st)
	})

	return state
}

func (c *CommandHistory) SetState(state *CommandHistoryState) {
	if state == nil {
		return
	}

	for _, scmd := range state.Cmds {
		e := &CommandHistoryEntry{
			cmd:     scmd.Cmd,
			started: scmd.Started,
			ended:   scmd.Ended,
			state:   scmd.State,
			dir:     scmd.Dir,
		}

		if e.state == Running {
			e.state = Orphaned
		}

		c.cmds.Add(e)
	}
}
