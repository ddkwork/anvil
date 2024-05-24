package main

import (
	"bytes"
	"fmt"
	"strings"
)

type helper struct {
	data map[string]string
}

var help = newHelp()

func newHelp() helper {
	h := helper{
		data: map[string]string{},
	}

	h.addTopics()

	return h
}

func (h *helper) addHelp(topic string, text string) {
	h.data[topic] = text
	h.data[strings.ToLower(topic)] = text
}

func (h helper) help(topic string) string {
	t, ok := h.data[topic]
	if !ok {
		return ""
	}
	return t
}

func AddHelp(topic string, text string) {
	help.addHelp(topic, text)
}

func Help(topic string) string {
	return help.help(topic)
}

func (h helper) addTopics() {
	s := fmt.Sprintf("%s is an editor strongly inspired by Rob Pike's Acme editor originally written for Plan 9 (see http://acme.cat-v.org/).",
		strings.Title(editorName))
	h.addHelp("Intro", s)

	s = `

The following environment variables are set for external commands:

ANVIL_WIN_LOCAL_PATH	The path of the file being edited in the window where the command was executed. This path does not include the hostname (if the command was being executed remotely).

ANVIL_WIN_GLOBAL_PATH	The path of the file being edited in the window where the command was executed including the hostname in the ssh format (i.e. HOST:PATH)

ANVIL_WIN_LOCAL_DIR	The parent directory of the file being edited in the window where the command was executed, or the file itself if it is a directory. This path does not include the hostname (if the command was being executed remotely).

ANVIL_WIN_GLOBAL_DIR	The parent directory of the file being edited in the window where the command was executed, or the file itself if it is a directory. This includes the hostname in the ssh format (i.e. HOST:PATH)

ANVIL_WIN_ID	The internal numeric ID of the window. This may be used in the API.

ANVIL_API_PORT	The TCP port number on which the Anvil REST API is running. Connections to the API should be performed to the local host; if a remote command is executed an SSH tunnel is created so that commands may connect locally.

ANVIL_API_SESS	Session id used to authenticate the client program against the API.
`
	h.addHelp("Environment", s)

	h.addRegexHelp()
}

func topLevelHelpString() string {
	var text bytes.Buffer
	fmt.Fprintf(&text,
		`Welcome to the %s editor. In this output, you can click the text delimited by ◊ characters to get help on specific topics. The help text will be appended to the end of this buffer, which you can scroll to by middle-clicking and dragging the scrollbar to the left of the window, or by typing CTRL-END.

Middle-click in ◊Help Intro◊ for a brief introduction of the editor.

=== Misc ===

This section lists help topics for various aspects of the editor.

Environment (◊Help Environment◊)
	Information about the environment variables set when external commands are run

Regex (◊Help Regex◊)
	Syntax of regular expressions

=== Commands ===

The following commands are built in.

`, strings.Title(editorName))

	return text.String()
}

func (h helper) addRegexHelp() {
	s := `

The syntax of regular expressions used in Anvil is the same as the regular expressions of the Go regexp package. A brief summary is printed here.

Single characters:

.              any character, possibly including newline (flag s=true)
[xyz]          character class
[^xyz]         negated character class
\d             Perl character class
\D             negated Perl character class
[[:alpha:]]    ASCII character class
[[:^alpha:]]   negated ASCII character class
\pN            Unicode character class (one-letter name)
\p{Greek}      Unicode character class
\PN            negated Unicode character class (one-letter name)
\P{Greek}      negated Unicode character class

Composites:

xy             x followed by y
x|y            x or y (prefer x)

Repetitions:

x*             zero or more x, prefer more
x+             one or more x, prefer more
x?             zero or one x, prefer one
x{n,m}         n or n+1 or ... or m x, prefer more
x{n,}          n or more x, prefer more
x{n}           exactly n x
x*?            zero or more x, prefer fewer
x+?            one or more x, prefer fewer
x??            zero or one x, prefer zero
x{n,m}?        n or n+1 or ... or m x, prefer fewer
x{n,}?         n or more x, prefer fewer
x{n}?          exactly n x

Grouping:

(re)           numbered capturing group (submatch)
(?P<name>re)   named & numbered capturing group (submatch)
(?:re)         non-capturing group
(?flags)       set flags within current group; non-capturing
(?flags:re)    set flags during re; non-capturing

Flag syntax is xyz (set) or -xyz (clear) or xy-z (set xy, clear z). The flags are:

i              case-insensitive (default false)
m              multi-line mode: ^ and $ match begin/end line in addition to begin/end text (default false)
s              let . match \n (default false)
U              ungreedy: swap meaning of x* and x*?, x+ and x+?, etc (default false)

Empty strings:

^              at beginning of text or line (flag m=true)
$              at end of text (like \z not \Z) or line (flag m=true)
\A             at beginning of text
\b             at ASCII word boundary (\w on one side and \W, \A, or \z on the other)
\B             not at ASCII word boundary
\z             at end of text

Escape sequences:

\a             bell (== \007)
\f             form feed (== \014)
\t             horizontal tab (== \011)
\n             newline (== \012)
\r             carriage return (== \015)
\v             vertical tab character (== \013)
\*             literal *, for any punctuation character *
\123           octal character code (up to three digits)
\x7F           hex character code (exactly two digits)
\x{10FFFF}     hex character code
\Q...\E        literal text ... even if ... has punctuation

Character class elements:

x              single character
A-Z            character range (inclusive)
\d             Perl character class
[:foo:]        ASCII character class foo
\p{Foo}        Unicode character class Foo
\pF            Unicode character class F (one-letter name)

Named character classes as character class elements:

[\d]           digits (== \d)
[^\d]          not digits (== \D)
[\D]           not digits (== \D)
[^\D]          not not digits (== \d)
[[:name:]]     named ASCII class inside character class (== [:name:])
[^[:name:]]    named ASCII class inside negated character class (== [:^name:])
[\p{Name}]     named Unicode property inside character class (== \p{Name})
[^\p{Name}]    named Unicode property inside negated character class (== \P{Name})

Perl character classes (all ASCII-only):

\d             digits (== [0-9])
\D             not digits (== [^0-9])
\s             whitespace (== [\t\n\f\r ])
\S             not whitespace (== [^\t\n\f\r ])
\w             word characters (== [0-9A-Za-z_])
\W             not word characters (== [^0-9A-Za-z_])

ASCII character classes:

[[:alnum:]]    alphanumeric (== [0-9A-Za-z])
[[:alpha:]]    alphabetic (== [A-Za-z])
[[:ascii:]]    ASCII (== [\x00-\x7F])
[[:blank:]]    blank (== [\t ])
[[:cntrl:]]    control (== [\x00-\x1F\x7F])
[[:digit:]]    digits (== [0-9])
[[:graph:]]    graphical (== [!-~] == [A-Za-z0-9!"#$%&'()*+,\-./:;<=>?@[\\\]^_` + "`" + `{|}~])
[[:lower:]]    lower case (== [a-z])
[[:print:]]    printable (== [ -~] == [ [:graph:]])
[[:punct:]]    punctuation (== [!-/:-@[-` + "`" + `{-~])
[[:space:]]    whitespace (== [\t\n\v\f\r ])
[[:upper:]]    upper case (== [A-Z])
[[:word:]]     word characters (== [0-9A-Za-z_])
[[:xdigit:]]   hex digit (== [0-9A-Fa-f])

Unicode character classes are those in unicode.Categories and unicode.Scripts.

`
	h.addHelp("Regex", s)
}