package main

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/jeffwilliams/anvil/internal/circ"
)

type CommandHistory struct {
	cmds circ.Circ[*CommandHistoryEntry]
	lock sync.Mutex
}

type CommandHistoryEntry struct {
	cmd         string
	started     time.Time
	ended       time.Time
	state       RunState
	dir         string
	exitCode    int
	exitCodeSet bool
}

type RunState int

const (
	Running RunState = iota
	Completed
	Orphaned
)

type Verbosity int

const (
	NotVerbose = iota
	Verbose
)

func (r RunState) String() string {
	switch r {
	case Running:
		return "running"
	case Completed:
		return "complete"
	case Orphaned:
		return "orphaned"
	default:
		return "unknown state"
	}
}

func NewCommandHistory(max int) *CommandHistory {
	return &CommandHistory{cmds: circ.New[*CommandHistoryEntry](max)}
}

func (ch *CommandHistory) Started(dir, cmd string) *CommandHistoryEntry {
	ch.lock.Lock()
	defer ch.lock.Unlock()
	e := &CommandHistoryEntry{
		cmd:     cmd,
		started: time.Now(),
		state:   Running,
		dir:     dir,
	}
	ch.cmds.Add(e)
	return e
}

func (ch *CommandHistory) Completed(e *CommandHistoryEntry) {
	ch.lock.Lock()
	defer ch.lock.Unlock()
	e.ended = time.Now()
	e.state = Completed
}

func (ch *CommandHistory) SetExitCode(e *CommandHistoryEntry, c int) {
	ch.lock.Lock()
	defer ch.lock.Unlock()
	e.exitCode = c
	e.exitCodeSet = true
}

func (ch *CommandHistory) String(verbosity Verbosity) string {
	var buf bytes.Buffer

	ch.lock.Lock()
	defer ch.lock.Unlock()
	ch.cmds.Each(func(e *CommandHistoryEntry) {
		ss, es := ch.formatTimes(e.started, e.ended)
		dirString := ""
		if verbosity == Verbose && e.dir != "" {
			dirString = fmt.Sprintf("On %s ", e.dir)
		}
		exitCode := ""
		if e.exitCodeSet {
			exitCode = fmt.Sprintf("(exit %d)", e.exitCode)
		}

		switch e.state {
		case Completed:
			fmt.Fprintf(&buf, "[%s – %s]%s %s%s\n", ss, es, exitCode, dirString, e.cmd)
		case Orphaned:
			fmt.Fprintf(&buf, "[%s %s] %s%s\n", ss, e.state, dirString, e.cmd)
		case Running:
			fmt.Fprintf(&buf, "[%s %s] %s%s\n", ss, e.state, dirString, e.cmd)
		default:
			fmt.Fprintf(&buf, "[? ?] %s%s\n", dirString, e.cmd)
		}
	})

	return buf.String()
}

func (ch *CommandHistory) formatTimes(start, end time.Time) (startStr, endStr string) {
	var format string

	if ch.sameDate(start, time.Now()) {
		format = "15:04:05"
	} else {
		format = "2006-01-02 15:04:05"
	}

	startStr = start.Format(format)

	if ch.sameDate(start, end) {
		format = "15:04:05"
	} else {
		format = "2006-01-02 15:04:05"
	}

	endStr = end.Format(format)
	return
}

func (ch *CommandHistory) sameDate(t1, t2 time.Time) bool {
	return t1.YearDay() == t2.YearDay()
}
