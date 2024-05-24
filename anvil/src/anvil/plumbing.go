package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ddkwork/golibrary/mylog"
)

/*
Plumbing file format:

		match <regex>
		do <command>

		match <regex>
		do <command>


<regex> can contain submatches which can be referenced by $0 (the entire match), $1 (first group), $2, etc.
<command> is an anvil or shell command.

*/

type Plumber struct {
	rules []PlumbingRule
}

func NewPlumber(rules []PlumbingRule) *Plumber {
	return &Plumber{
		rules,
	}
}

func (p Plumber) Plumb(obj string, executor *CommandExecutor, ctx *CmdContext) (ok bool, err error) {
	for _, rule := range p.rules {
		if rule.Try(obj, executor, ctx) {
			ok = true
			return
		}
	}
	return false, nil
}

type PlumbingRule struct {
	Match *regexp.Regexp
	Do    string
}

func (rule PlumbingRule) Try(obj string, executor *CommandExecutor, ctx *CmdContext) (matched bool) {
	submatches := rule.Match.FindStringSubmatchIndex(obj)
	if submatches == nil {
		return
	}

	matched = true
	cmd := []byte{}
	cmd = rule.Match.Expand(cmd, []byte(rule.Do), []byte(obj), submatches)

	log(LogCatgPlumb, "Plumber: executing '%s'\n", cmd)

	executor.Do(string(cmd), ctx)
	return
}

func ParsePlumbingRules(f io.Reader) (rules []PlumbingRule) {
	s := bufio.NewScanner(f)

	const (
		stateExpectMatch = iota
		stateExpectDo
	)

	state := stateExpectMatch

	var rule PlumbingRule

	for s.Scan() {
		line := s.Text()
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}

		toks := strings.SplitN(line, " ", 2)
		if len(toks) < 2 {
			mylog.Check(fmt.Errorf("invalid line: expected a word, a space, then a string. Line is '%s'", line))
		}

		switch state {
		case stateExpectMatch:
			if toks[0] != "match" {
				mylog.Check(fmt.Errorf("expected line beginning with 'match' but got '%s'", line))
			}
			rule.Match = mylog.Check2(regexp.Compile(toks[1]))
			state = stateExpectDo
		case stateExpectDo:
			if toks[0] != "do" {
				mylog.Check(fmt.Errorf("expected line beginning with 'do' but got '%s'", line))
			}
			rule.Do = toks[1]
			rules = append(rules, rule)
			state = stateExpectMatch
		}
	}
	return
}
