package main

import (
	"anvil-go-api"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/acarl005/stripansi"
	"github.com/ddkwork/golibrary/mylog"
	"github.com/ogier/pflag"
)

var (
	noBody       io.Reader
	anvil        api.Anvil
	ttyWinId     int
	isTerminated func() bool
	doDebug      = true
)

var optDebug = pflag.BoolP("debug", "d", false, "Print debug messages")

func debug(format string, args ...interface{}) {
	if !doDebug {
		return
	}
	fmt.Printf(format, args...)
}

func main() {
	parseOpts()

	cmdArgv := mylog.Check2(commandAndArgsToRun())

	cmdStdin, cmdStdout, f := mylog.Check4(startCmd(cmdArgv))
	isTerminated = f
	dieIfError(err, fmt.Sprintf("awin: Starting command failed: %v\n", err))

	anvilSess := getEnvOrDie("ANVIL_API_SESS")
	anvilPort := getEnvOrDie("ANVIL_API_PORT")
	anvilGlobalPath := os.Getenv("ANVIL_WIN_GLOBAL_PATH")
	if anvilGlobalPath == "" {

		anvilGlobalPath = mylog.Check2(os.Getwd())
		dieIfError(err, fmt.Sprintf("awin: Environment variable ANVIL_WIN_GLOBAL_PATH is not set and getting current dir failed"))
	}

	anvil = api.New(anvilSess, anvilPort)

	registerSendCommand(&anvil)

	compoundPath := compoundPathForTag(anvilGlobalPath, cmdArgv)
	win := findOrCreateWindow(&anvil, compoundPath)
	ttyWinId = win.Id

	notifChan, lastLineChan, clearLastLineChan, procOutputChan := setupPlumbing()

	go readNotifs(notifChan)
	go readProcess(cmdStdout, procOutputChan)
	np := NewNotificationProcessor(cmdStdin, notifChan, lastLineChan, clearLastLineChan)
	go np.run()
	oh := NewProcessOutputHandler(ttyWinId, procOutputChan, lastLineChan, clearLastLineChan)
	oh.run()
}

func setupPlumbing() (
	notifChan chan []api.Notification,
	lastLineChan chan string,
	clearLastLineChan chan struct{},
	procOutputChan chan []byte,
) {
	notifChan = make(chan []api.Notification)
	lastLineChan = make(chan string)
	clearLastLineChan = make(chan struct{})
	procOutputChan = make(chan []byte)
	return
}

func parseOpts() {
	pflag.Parse()
	doDebug = *optDebug
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <command> [argument...]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Run an interactive command-line process inside an Anvil window.\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")

	pflag.PrintDefaults()
}

// readNotifs reads notifications from Anvil and writes them to the channel c.
func readNotifs(c chan<- []api.Notification) {
	var notifs []api.Notification

	for {
		anvil.GetInto("/notifs", &notifs)
		c <- notifs
		time.Sleep(1 * time.Second)
	}
}

type NotificationProcessor struct {
	lastLineFromProcess string
	cmdStdin            io.Writer
	notifChan           <-chan []api.Notification
	lastLineChan        <-chan string
	clearLastLineChan   chan<- struct{}
}

func NewNotificationProcessor(cmdStdin io.Writer, nc <-chan []api.Notification,
	lastLineChan <-chan string, clearLastLineChan chan<- struct{},
) NotificationProcessor {
	return NotificationProcessor{
		cmdStdin:          cmdStdin,
		notifChan:         nc,
		lastLineChan:      lastLineChan,
		clearLastLineChan: clearLastLineChan,
	}
}

func (p *NotificationProcessor) run() {
	for {
		select {
		case notifs := <-p.notifChan:
			p.processExecNotifs(notifs)
			p.processBodyChangeNotifs(notifs)
		case l := <-p.lastLineChan:
			p.lastLineFromProcess = l
		}
	}
}

func (p *NotificationProcessor) processExecNotifs(notifs []api.Notification) {
	for _, n := range notifs {
		if n.Op == api.NotificationOpExec {
			// fmt.Printf("Got command notification: %#+v\n", n)
			if n.WinId != ttyWinId {
				continue
			}

			if n.Cmd[0] != "Send" {
				continue
			}

			p.processSendNotification(n)
		}
	}
}

func (p *NotificationProcessor) processSendNotification(n api.Notification) {
	if len(n.Cmd) > 1 {
		cmd := strings.Join(n.Cmd[1:], " ")
		debug("awin: sending to process: '%s' (%v)\n", cmd, []byte(cmd))
		fmt.Fprintf(p.cmdStdin, "%s\r", cmd)
		return
	}
}

func (p *NotificationProcessor) processBodyChangeNotifs(notifs []api.Notification) {
	var info api.WindowBody
	anvil.GetInto(fmt.Sprintf("/wins/%d/body/info", ttyWinId), &info)

	isAppend := doNotifsContainAnAppend(notifs, info.Len)
	if !isAppend {
		return
	}

	rsp := mylog.Check2(anvil.Get(fmt.Sprintf("/wins/%d/body", ttyWinId)))
	dieIfError(err, fmt.Sprintf("awin: Error reading window body"))
	body := mylog.Check2(ioutil.ReadAll(rsp.Body))
	dieIfError(err, fmt.Sprintf("awin: Error reading window body"))

	pl := p.lastLineFromProcess
	l := promptOrLastFullLine(string(body))

	if !endsWithByte(l, '\n') {
		return
	}

	if pl == l {
		return
	}

	l = stripPrompt(l, pl)

	if l[len(l)-1] == '\n' {
		l = l[0:len(l)-1] + "\r"
	}
	debug("awin: sending to process: '%s' (%v)\n", l, []byte(l))
	fmt.Fprintf(p.cmdStdin, "%s", l)
	p.clearLastLineChan <- struct{}{}
}

func readProcess(cmdStdout io.Reader, c chan<- []byte) {
	buf := make([]byte, 100)

	for {
		n := mylog.Check2(cmdStdout.Read(buf))

		b := make([]byte, n)
		copy(b, buf[0:n])
		c <- b
	}

	close(c)
}

type ProcessOutputHandler struct {
	lastLine          bytes.Buffer
	procOutput        <-chan []byte
	lastLineChan      chan<- string
	clearLastLineChan <-chan struct{}
	winId             int
}

func NewProcessOutputHandler(winId int, procOutput <-chan []byte, lastLineChan chan<- string, clearLastLineChan <-chan struct{}) ProcessOutputHandler {
	return ProcessOutputHandler{
		winId:             winId,
		procOutput:        procOutput,
		lastLineChan:      lastLineChan,
		clearLastLineChan: clearLastLineChan,
	}
}

func (p *ProcessOutputHandler) run() {
	for {
		select {
		case buf, ok := <-p.procOutput:
			if !ok {
				return
			}
			p.process(buf)
		case <-p.clearLastLineChan:
			p.lastLine.Reset()
		}
	}
}

func (p *ProcessOutputHandler) process(buf []byte) {
	for i, b := range buf {
		if b == '\n' && i < len(buf) {
			p.lastLine.Reset()
		} else {
			p.lastLine.WriteByte(b)
		}
	}

	lastLineFromProcess := p.clean(p.lastLine.Bytes())
	p.lastLineChan <- lastLineFromProcess

	cleaned := p.clean(buf)
	debug("awin: output from process: '%s'\n", cleaned)
	debug("awin: last line from process: '%s'\n", lastLineFromProcess)
	p.appendToWindowBody([]byte(cleaned))
	p.moveCursorToEndOfBody()
}

func (p *ProcessOutputHandler) clean(buf []byte) string {
	cleaned := strings.ReplaceAll(string(buf), "\r\n", "\n")
	cleaned = stripansi.Strip(cleaned)
	return cleaned
}

func (p *ProcessOutputHandler) appendToWindowBody(buf []byte) {
	r := bytes.NewReader(buf)
	anvil.Post(fmt.Sprintf("/wins/%d/body", p.winId), r)
}

func (p *ProcessOutputHandler) moveCursorToEndOfBody() {
	var info api.WindowBody
	anvil.GetInto(fmt.Sprintf("/wins/%d/body/info", p.winId), &info)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "[%d]", info.Len)
	anvil.Put(fmt.Sprintf("/wins/%d/body/cursors", p.winId), &buf)
}

func endsWithByte(s string, b byte) bool {
	if s == "" {
		return false
	}

	return s[len(s)-1] == b
}

func stripPrompt(s, prompt string) string {
	if !endsWithByte(prompt, '\n') && strings.HasPrefix(s, prompt) {
		s = s[len(prompt):]
	}
	return s
}

func registerSendCommand(anvil *api.Anvil) {
	debug("awin: Registering Send command\n")
	var buf bytes.Buffer
	buf.WriteString(`["Send"]`)
	anvil.Post("/cmds", &buf)
	debug("awin: Done registering Send command\n")
}

func findOrCreateWindow(anvil *api.Anvil, compoundPath string) api.Window {
	var wins []api.Window
	mylog.Check(anvil.GetInto("/wins", &wins))
	dieIfError(err, fmt.Sprintf("awin: "))
	for _, w := range wins {
		if w.Path == compoundPath {
			return w
		}
	}

	win := createNewWindow(anvil)
	setWindowTag(anvil, win.Id, compoundPath)
	return win
}

func createNewWindow(anvil *api.Anvil) api.Window {
	debug("awin: Creating new window\n")
	rsp := mylog.Check2(anvil.Post("/wins", noBody))
	dieIfError(err, fmt.Sprintf("awin: "))
	debug("awin: Done creating new window\n")

	raw := mylog.Check2(ioutil.ReadAll(rsp.Body))
	dieIfError(err, fmt.Sprintf("awin: Error reading response body in POST to /wins"))

	var win api.Window
	mylog.Check(json.Unmarshal(raw, &win))
	dieIfError(err, fmt.Sprintf("awin: Error decoding JSON response body in POST to /wins"))
	debug("New window id: %d\n", win.Id)
	return win
}

func setWindowTag(anvil *api.Anvil, winId int, compoundPath string) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s Del! Snarf | Look  Send ", compoundPath)
	anvil.Put(fmt.Sprintf("/wins/%d/tag", winId), &buf)
}

func compoundPathForTag(winPath string, argv []string) string {
	cmd := ""
	if len(argv) > 0 {
		cmd = argv[0]
	}

	return fmt.Sprintf("%s-%s", winPath, cmd)
}

var (
	lastLineFromProcess string
	lock                sync.Mutex
)

func doNotifsContainAnAppend(notifs []api.Notification, bodyLen int) bool {
	for _, notif := range notifs {
		if notif.WinId != ttyWinId {
			continue
		}

		if notif.Op == api.NotificationOpInsert && notif.Offset+notif.Len == bodyLen {
			return true
		}
	}

	return false
}

func getEnvOrDie(name string) string {
	v := os.Getenv(name)
	if v == "" {
		fmt.Fprintf(os.Stderr, "awin: Environment variable %s is not set\n", name)
		os.Exit(1)
	}

	return v
}

func dieIfError(err error, msg string) {
}

func lastLineIn(s string) string {
	newlines := 0
	start := 0
	end := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			newlines++
			if newlines == 1 {
				end = i
			} else if newlines == 2 {
				start = i + 1
				break
			}
		}
	}

	if start >= 0 && end >= 0 && start != end {
		return s[start:end]
	}

	return ""
}

func promptOrLastFullLine(s string) string {
	if s == "" {
		return ""
	}

	if l := len(s) - 1; s[l] == '\n' {
		return textAfterLastNewline(s[:l]) + "\n"
	}
	return textAfterLastNewline(s)
}

func textAfterLastNewline(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return s[i+1:]
		}
	}
	return s
}

func commandAndArgsToRun() (argv []string, err error) {
	if len(pflag.Args()) < 1 {
		mylog.Check(fmt.Errorf("No command specified"))
		return
	}

	argv = pflag.Args()
	return
}
