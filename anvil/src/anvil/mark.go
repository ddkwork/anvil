package main

import (
	"bytes"
	"fmt"
	"sort"
)

type Marks struct {
	marks map[string]*MarkPosition
}

type MarkPosition struct {
	FileName string
	Index    int
}

func (m *Marks) Set(markName, fileName string, index int) {
	if m.marks == nil {
		m.marks = make(map[string]*MarkPosition)
	}
	m.marks[markName] = &MarkPosition{fileName, index}
}

func (m *Marks) Unset(name string) {
	if m.marks == nil {
		return
	}
	delete(m.marks, name)
}

func (m *Marks) Clear() {
	if m.marks == nil {
		return
	}
	m.marks = make(map[string]*MarkPosition)
}

func (m *Marks) Seek(name string) (fileName string, goTo seek, ok bool) {
	if m.marks == nil {
		return
	}

	var pos *MarkPosition
	pos, ok = m.marks[name]
	if !ok {
		return
	}

	fileName = pos.FileName
	goTo = seek{
		seekType: seekToRunePos,
		runePos:  pos.Index,
	}

	return
}

func (m *Marks) String() string {
	if m.marks == nil {
		return ""
	}

	keys := make([][2]string, len(m.marks))

	i := 0
	for k, v := range m.marks {
		keys[i][0] = k
		keys[i][1] = fmt.Sprintf("%s#%d", v.FileName, v.Index)
		i++
	}

	sort.Slice(keys, func(a, b int) bool {
		return keys[a][0] < keys[b][0]
	})

	var buf bytes.Buffer
	for _, v := range keys {
		fmt.Fprintf(&buf, "Goto %s\n\t%s\n", v[0], v[1])
	}

	return buf.String()
}

type MarkState struct {
	Marks map[string]*MarkPosition
}

func (m *Marks) State() MarkState {
	return MarkState{
		Marks: m.marks,
	}
}

func (m *Marks) SetState(state MarkState) {
	m.marks = state.Marks
}

func (m *Marks) ShiftDueToTextModification(fileName string, startOfChange, lengthOfChange int) {
	for _, mark := range m.marks {
		if mark.FileName == fileName {
			if mark.Index >= startOfChange {
				mark.Index += lengthOfChange
			}
		}
	}
}
