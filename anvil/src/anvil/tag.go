package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ddkwork/golibrary/mylog"
)

type Tag struct {
	blockEditable
	flash bool
}

func (t *Tag) Init(body *Body, style blockStyle, editableStyle editableStyle, executor *CommandExecutor, finder *FileFinder, owner interface{}, scheduler *Scheduler) {
	t.blockEditable.Init(style, editableStyle, scheduler)
	t.executeOn = &t.editable
	if body != nil {
		t.executeOn = &body.editable
	}
	t.PreventScrolling = true
	t.SetAdapter(&editableAdapter{
		fileFinder: finder,
		executor:   executor,
		owner:      owner,
	})
	t.AddTextChangeListener(t.highlightBasenameOnTextChange)
}

func (t Tag) Parts() (path, editorArea, userArea string, err error) {
	parts, _ := mylog.Check3(t.calcParts())
	s := t.String()
	path = s[parts.path[0]:parts.path[1]]
	editorArea = s[parts.editorArea[0]:parts.editorArea[1]]
	userArea = s[parts.userArea[0]:parts.userArea[1]]
	return
}

func (t *Tag) Set(path, editorArea, userArea string) {
	pathLen := utf8.RuneCountInString(path)
	editorAreaLen := utf8.RuneCountInString(editorArea)

	t.SetTextString(fmt.Sprintf("%s%s%s", path, editorArea, userArea))

	t.immutableRange.start = pathLen
	t.immutableRange.end = editorAreaLen + pathLen

	t.setBgColor(path)
	t.ClearManualHighlights()
	t.highlightBasename(path)
}

func (t *Tag) setBgColor(path string) {
	if strings.HasSuffix(path, "+Errors") {
		if t.flash {
			t.blockEditable.bgcolor = t.blockEditable.style.ErrorFlashBgColor
		} else {
			t.blockEditable.bgcolor = t.blockEditable.style.ErrorBgColor
		}
	} else {
		t.blockEditable.bgcolor = t.blockEditable.style.StandardBgColor
	}
}

type tagParts struct {
	path       section
	editorArea section
	userArea   section
}

type section [2]int

func (sec section) Section(s string) string {
	return s[sec[0]:sec[1]]
}

func (t Tag) calcParts() (inBytes tagParts, inRunes tagParts, err error) {
	s := t.String()

	return calculateTagParts(s)
}

func calculateTagParts(tag string) (inBytes tagParts, inRunes tagParts, err error) {
	if tag == "" {
		return
	}

	i := strings.IndexRune(tag, '|')
	if i < 0 {
		mylog.Check(fmt.Errorf("Tag does not contain |"))
		return
	}
	inBytes.userArea[0] = i + 1
	inBytes.userArea[1] = len(tag)

	j := strings.LastIndex(tag[:i], " Del")
	if j < 0 {
		mylog.Check(fmt.Errorf("Tag does not contain ' Del'"))
		return
	}

	inBytes.editorArea[0] = j
	inBytes.editorArea[1] = i + 1

	inBytes.path[0] = 0
	inBytes.path[1] = j

	part := inBytes.path.Section(tag)
	inRunes.path[1] = utf8.RuneCountInString(part)

	part = inBytes.editorArea.Section(tag)
	inRunes.editorArea[0] = inRunes.path[1]
	inRunes.editorArea[1] = utf8.RuneCountInString(part) + inRunes.editorArea[0]

	part = inBytes.userArea.Section(tag)
	inRunes.userArea[0] = inRunes.editorArea[1]
	inRunes.userArea[1] = utf8.RuneCountInString(part) + inRunes.userArea[0]

	return
}

func (t *Tag) SetStyle(style blockStyle, editableStyle editableStyle) {
	t.blockEditable.SetStyle(style, editableStyle)
	path, _, _, _ := t.Parts()
	t.setBgColor(path)
}

func (t *Tag) SetFlash(b bool) {
	t.flash = b
	path, _, _, _ := t.Parts()
	t.setBgColor(path)
}

func (t *Tag) highlightBasename(path string) {
	pathLen := utf8.RuneCountInString(path)

	g := mylog.Check2(NewGlobalPath(path, GlobalPathUnknown))

	b := g.Base()
	baseLen := utf8.RuneCountInString(b)

	start := pathLen - baseLen
	if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		start--
	}
	end := t.immutableRange.start + 1
	log(LogCatgEd, "Tag.highlightBasename: highlighting basename of path %s, between %d and %d", path, start, end)

	t.AddManualHighlight(start, end, Color(t.style.PathBasenameColor))
}

func (t *Tag) highlightBasenameOnTextChange(ch *TextChange) {
	t.ClearManualHighlights()
	path, _, _ := mylog.Check4(t.Parts())
	t.highlightBasename(path)
}
