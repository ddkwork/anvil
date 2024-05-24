package main

import (
	"fmt"
	"sort"

	"github.com/jeffwilliams/anvil/internal/intvl"
	"github.com/jeffwilliams/anvil/internal/runes"
	"github.com/jeffwilliams/anvil/internal/slice"
)

// selection represents a selected segment of text between [start, end). TODO: Rename this to textrange.
type selection struct {
	start, end int
}

type selectionPurpose int

const (
	SelectionPurposeSelect selectionPurpose = iota
	SelectionPurposeExecute
)

func NewSelection(start, end int) selection {
	s := selection{start, end}
	s.Reorient()
	return s
}

func (s selection) String() string {
	return fmt.Sprintf("[%d,%d)", s.start, s.end)
}

func (s selection) Start() int {
	return s.start
}

func (s selection) End() int {
	return s.end
}

func (s selection) Overlaps(o *selection) bool {
	return intvl.Overlaps(s, o)
}

func (s *selection) Reorient() {
	if s.end < s.start {
		s.start, s.end = s.end, s.start
	}
}

func (s *selection) Len() int {
	return s.end - s.start
}

func (e *editableModel) clearSelections() {
	if e.selections != nil {
		e.selections = e.selections[:0]
	}
	e.primarySel = nil
	e.primarySelPurpose = SelectionPurposeSelect
	e.selectionBeingBuilt = nil
}

func (e *editable) clearSelections() {
	e.editableModel.clearSelections()
	editor.clearLastSelectionIfOwnedBy(e)
}

func (e *editableModel) addSelection(s *selection) {
	var remove []*selection
	for _, o := range e.selections {
		if o.Overlaps(s) {
			remove = append(remove, o)
		}
	}

	for _, o := range remove {
		e.removeSelection(o)
	}

	e.selections = append(e.selections, s)
	e.selectionsModified()
}

func (e *editableModel) numberOfSelections() int {
	return len(e.selections)
}

func (e *editableModel) SelectionsPresent() bool {
	return e.numberOfSelections() > 0
}

func (e *editable) selectionStartingFirst() *selection {
	if len(e.selections) == 0 {
		return nil
	}

	min := e.selections[0]
	for _, s := range e.selections {
		if s.start < min.start {
			min = s
		}
	}
	return min
}

func (e *editableModel) removeSelection(sel *selection) {
	match := func(i int) bool {
		return e.selections[i] == sel
	}
	s := slice.RemoveFirstMatchFromSlice(e.selections, match)
	e.selections = s.([]*selection)
	e.selectionsModified()
}

func (e *editable) extendSelectionBeingBuilt(rank SelectionRank, index int) {
	if e.selectionBeingBuilt == nil {
		e.startBuildingSelection(rank)
	}

	ci := e.lastCursorIndex()

	if index <= ci {
		e.selectionBeingBuilt.start = index
		e.selectionBeingBuilt.end = ci
	} else {
		e.selectionBeingBuilt.start = ci
		e.selectionBeingBuilt.end = index
	}
	e.selectionsModified()
}

func (e *editable) startBuildingSelection(rank SelectionRank) {
	sel := &selection{start: e.firstCursorIndex(), end: e.firstCursorIndex()}
	e.addSelection(sel)
	e.selectionBeingBuilt = sel
	if rank == PrimarySelection {
		e.primarySel = sel
		e.primarySelPurpose = SelectionPurposeSelect
		editor.setLastSelection(e, e.primarySel)
	}
}

func (e *editable) setPrimarySelection(start, end int) {
	if e.primarySel != nil {
		e.primarySel.start = start
		e.primarySel.end = end
		editor.setLastSelection(e, e.primarySel)
		return
	}

	e.addPrimarySelection(start, end)
}

func (e *editable) addPrimarySelection(start, end int) {
	sel := &selection{start: start, end: end}
	e.addSelection(sel)
	e.primarySel = sel
	e.primarySelPurpose = SelectionPurposeSelect
	editor.setLastSelection(e, sel)
	e.selectionsModified()
}

func (e *editableModel) selectionsModified() {
	e.typingInSelectedTextAction = replaceSelectionsWithText
}

func (e *editableModel) addSecondarySelection(start, end int) (setPrimary bool) {
	// TODO: check for overlaps
	sel := &selection{start: start, end: end}
	e.addSelection(sel)
	if e.primarySel == nil {
		e.primarySel = sel
		setPrimary = true
	}
	return
}

func (e *editable) addSecondarySelection(start, end int) {
	setPrimary := e.editableModel.addSecondarySelection(start, end)
	if setPrimary {
		editor.setLastSelection(e, e.primarySel)
	}
}

func (e *editableModel) selectionContaining(runeIndex int) *selection {
	for _, s := range e.selections {
		if s.start <= runeIndex && s.end > runeIndex {
			return s
		}
	}
	return nil
}

func (e *editable) dumpSelections() {
	sort.Slice(e.selections, func(a, b int) bool {
		return e.selections[a].start < e.selections[b].start
	})

	for _, sel := range e.selections {
		if e.primarySel == sel {
			log(LogCatgEd, "primary selection [%d,%d]\n", sel.start, sel.end)
		} else {
			log(LogCatgEd, "secondary selection [%d,%d]\n", sel.start, sel.end)
		}
	}
}

func (e *editableModel) textOfSelection(s *selection) string {
	w := runes.NewWalker(e.Bytes())
	if s != nil {
		return string(w.TextBetweenRuneIndices(s.start, s.end))
	}
	return ""
}

func (e *editable) textOfPrimarySelection() (text string, ok bool) {
	if e.primarySel == nil {
		return
	}

	text = e.textOfSelection(e.primarySel)
	ok = true
	return
}

func (e *editableModel) shiftSelectionsDueToTextModification(startOfChange, lengthOfChange int) {
	for _, s := range e.selections {
		e.shiftSelectionDueToTextModification(s, startOfChange, lengthOfChange)
	}

	if e.immutableRange.Len() != 0 {
		// We know that the user can't be typing within the immutable range, so they must be instead typing just
		// before the beginning of the immutable range
		e.shiftSelectionDueToTextModificationBounds(&e.immutableRange, startOfChange, lengthOfChange, changeAtBoundsIsNotWithinSelection)
	}
}

func (e *editableModel) shiftSelectionDueToTextModification(s *selection, startOfChange, lengthOfChange int) {
	s.start, s.end = computeShiftNeededDueToTextModification(s, startOfChange, lengthOfChange)
}

func (e *editableModel) shiftSelectionDueToTextModificationBounds(s *selection, startOfChange, lengthOfChange int, b changeAtSelectionBoundsBehaviour) {
	s.start, s.end = computeShiftNeededDueToTextModificationBounds(s, startOfChange, lengthOfChange, b)
}

func (e *editable) stopBuildingSelection() {
	e.selectionBeingBuilt = nil

	newSel := make([]*selection, 0, len(e.selections))

	for _, s := range e.selections {
		if s.Len() == 0 {
			continue
		}
		newSel = append(newSel, s)
	}
	e.selections = newSel
}

type SelectionRank int

const (
	PrimarySelection SelectionRank = iota
	SecondarySelection
)

func (e *editable) copySelections() []*selection {
	rc := make([]*selection, len(e.selections))

	for i, s := range e.selections {
		newS := &selection{}
		*newS = *s
		rc[i] = newS
	}
	return rc
}

func (e *editable) selectionsInDisplayOrder() []*selection {
	ordered := make([]*selection, len(e.selections))
	copy(ordered, e.selections)

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Start() < ordered[j].Start()
	})

	return ordered
}

func (e *editable) contractSelectionsOnLeftBy(amt int) {
	for i, s := range e.selections {
		if s.Len() < amt+1 {
			continue
		}

		e.selections[i].start += amt
	}
}

func (e *editable) selectAll() {
	e.clearSelections()
	e.setPrimarySelection(0, e.text.Len())
}
