package main

import (
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
)

type PointerState struct {
	pressedButtons      pointer.Buttons
	currentPointerEvent pointerEvent
	lastPointerEvent    pointerEvent
	lastPressEvent      pointerEvent
	handlers            map[PointerEventMatch]PointerEventHandler
	// consecutiveClicks is the number of consecutive clicks on the same button within a small duration
	consecutiveClicks int
	gtx               layout.Context
}

type pointerEvent struct {
	runeIndex int
	set       bool
	pointer.Event
	button pointer.Buttons
}

type PointerEventMatch struct {
	typ    pointer.Type
	button pointer.Buttons
}

func (ps *PointerState) Event(ev *pointer.Event, gtx layout.Context) {
	ps.currentPointerEvent.Event = *ev
	ps.currentPointerEvent.set = true
	ps.currentPointerEvent.button = ps.pointerButtonJustManipulated()
	ps.gtx = gtx
}

func (ps *PointerState) SetRuneIndexOfCurrentEvent(runeIndex int) {
	ps.currentPointerEvent.runeIndex = runeIndex
}

func (ps *PointerState) LenHandlers() int {
	return len(ps.handlers)
}

func (ps *PointerState) Handler(m PointerEventMatch, handler PointerEventHandler) {
	if ps.handlers == nil {
		ps.handlers = make(map[PointerEventMatch]PointerEventHandler)
	}
	ps.handlers[m] = handler
}

func (ps *PointerState) InvokeHandlers() {
	if ps.handlers == nil || !ps.currentPointerEvent.set {
		return
	}

	if ps.isZeroDistanceDrag() || ps.isTinyDistanceDrag() {
		return
	}

	// b := ps.pointerButtonJustManipulated()
	b := ps.currentPointerEvent.button
	m := PointerEventMatch{
		typ:    ps.currentPointerEvent.Type,
		button: b,
	}

	ps.updateConsecutiveClicks()

	if fn, ok := ps.handlers[m]; ok {
		fn(ps)
	}

	ps.updateHistoricalEvents()
	ps.pressedButtons = ps.currentPointerEvent.Buttons
}

func (ps *PointerState) isZeroDistanceDrag() bool {
	return ps.currentPointerEvent.Type == pointer.Drag && ps.currentPointerEvent.Position == ps.lastPointerEvent.Position
}

func (ps *PointerState) isTinyDistanceDrag() bool {
	return ps.currentPointerEvent.Type == pointer.Drag && pointerEventsOccurredAtAlmostSamePlace(&ps.lastPointerEvent, &ps.currentPointerEvent)
}

func (ps *PointerState) updateHistoricalEvents() {
	if ps.currentPointerEvent.Event.Type == pointer.Press {
		ps.lastPressEvent = ps.currentPointerEvent
	}
	ps.lastPointerEvent = ps.currentPointerEvent
}

func (ps *PointerState) updateConsecutiveClicks() {
	if ps.currentPointerEvent.Type != pointer.Press {
		return
	}

	if ps.lastPointerEvent.Type == pointer.Release &&
		ps.lastPointerEvent.button == ps.currentPointerEvent.button &&
		pointerEventsOccurredAtAlmostSamePlace(&ps.lastPointerEvent, &ps.currentPointerEvent) &&
		ps.currentPointerEvent.Time-ps.lastPointerEvent.Time < 250*time.Millisecond {

		ps.consecutiveClicks++
		return
	}

	ps.consecutiveClicks = 1
}

const (
	pointerEventDistanceTolerance        = 4 // In pixels
	pointerEventDistanceToleranceSquared = pointerEventDistanceTolerance * pointerEventDistanceTolerance
)

func pointerEventsOccurredAtAlmostSamePlace(a, b *pointerEvent) bool {
	return gioPointerEventsOccurredAtAlmostSamePlace(&a.Event, &b.Event)
}

func gioPointerEventsOccurredAtAlmostSamePlace(a, b *pointer.Event) bool {
	return pointsAreAlmostSame(&a.Position, &b.Position, pointerEventDistanceToleranceSquared)
}

func pointsAreAlmostSame(a, b *f32.Point, tolerance float32) bool {
	d := b.Sub(*a)
	return (d.X*d.X + d.Y*d.Y) < tolerance
}

func (ps *PointerState) SetConsecutiveClicks(n int) {
	ps.consecutiveClicks = n
}

type PointerEventHandler func(ps *PointerState)

func (ps PointerState) pointerButtonJustManipulated() pointer.Buttons {
	ev := &ps.currentPointerEvent.Event
	if ev.Type == pointer.Press {
		return ev.Buttons & (^ps.pressedButtons)
	} else if ev.Type == pointer.Release {
		return ps.pressedButtons ^ ev.Buttons
	} else if ev.Type == pointer.Drag {
		return ev.Buttons
	}
	return pointer.Buttons(0)
}

func (ps *PointerState) FreeLayoutContext() {
	ps.gtx = layout.Context{}
}
