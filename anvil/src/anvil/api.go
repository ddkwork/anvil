package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"unicode"

	"gioui.org/layout"
	"github.com/ddkwork/golibrary/mylog"
	"github.com/jszwec/csvutil"
)

/*
This file implements the Anvil REST API.

Summary of operations:


    GET /wins/: list window ids and paths
   POST /wins/: create a new window and return the id
    GET /wins/1/body: Get contents of body of window 1
    PUT /wins/1/body: Set contents of body of window 1
	 POST /wins/1/body: Append to the contents of the body of window 1
    GET /wins/1/body/info: Get info about window body (i.e. length)
    PUT /wins/1/body?start=20&end=25: Set part of buffer in [20,25). Not implemented.
    GET /wins/1/body/cursors: Get info about cursors in the window body
    PUT /wins/1/body/cursors: Set position of cursors in the window body
    GET /wins/1/info: get window information, such as file paths
    GET /wins/1/selections: get window selections
    GET /wins/1/tag: Get tag
    PUT /wins/1/tag: Set tag
    GET /jobs: list jobs
    GET /notifs: Get any pending notifications for the current API session. The notifications are then cleared.
	 POST /cmds: Create a new client-defined command. If it already exists, register interest in it.

	 POST /execute: Execute a command as if it was clicked. The command is executed as if it was run from the editor tag

		Supports JSON and CSV encodings. CSV is better for bash.
*/

var localApiPort int

func ServeLocalAPI() {
	l := mylog.Check2(net.Listen("tcp", "127.0.0.1:0"))
	tl, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		mylog.Check("listener is not a *net.TCPAddr. Can't determine port.")
	}
	localApiPort = tl.Port
	mylog.Check(ServeAPIOnListener(l))
}

func ServeAPIOnListener(l net.Listener) error {
	handler := &ApiHandler{}
	mylog.Check(http.Serve(l, handler))
	return nil
}

func LocalAPIPort() int {
	return localApiPort
}

type ApiHandler struct{}

func (a ApiHandler) ServeHTTP(rsp http.ResponseWriter, req *http.Request) {
	log(LogCatgAPI, "APIHandler.ServeHTTP: Received %s for URL path: %s\n", req.Method, req.URL.Path)

	sess, ok := a.authenticate(rsp, req)
	if !ok {
		return
	}

	if req.URL.Path == "/wins" {
		a.serveWindows(rsp, req)
		return
	} else if strings.HasPrefix(req.URL.Path, "/wins/") {
		winId, subpath := a.parseInitialNumber(req.URL.Path[6:])
		log(LogCatgAPI, "winId: %d subpath: %s\n", winId, subpath)

		switch subpath {
		case "/body":
			fallthrough
		case "/body/cursors":
			fallthrough
		case "/body/info":
			a.serveWindowBody(winId, rsp, req, subpath)
			return
		case "/selections":
			a.serveWindowSelections(winId, rsp, req)
			return
		case "/info":
			a.serveWindowInfo(winId, rsp, req)
			return
		case "/tag":
			a.serveWindowTag(winId, rsp, req)
			return
		}
	} else if req.URL.Path == "/jobs" {
		a.serveJobs(rsp, req)
		return
	} else if req.URL.Path == "/notifs" {
		a.serveNotifs(&sess, rsp, req)
		return
	} else if req.URL.Path == "/cmds" {
		a.serveCmds(&sess, rsp, req)
		return
	} else if req.URL.Path == "/execute" {
		a.serveExecute(&sess, rsp, req)
		return
	}

	// if strings.HasPrefix(req.URL.Path, "/wins"
	msg := fmt.Sprintf("Unsupported URL %s", req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) authenticate(rsp http.ResponseWriter, req *http.Request) (sess ApiSession, ok bool) {
	sess, ok = a.findSession(req)
	if !ok {
		msg := fmt.Sprintf("Anvil-Sess header is missing or invalid")
		http.Error(rsp, msg, http.StatusUnauthorized)
		return
	}
	return
}

func (a ApiHandler) findSession(req *http.Request) (sess ApiSession, ok bool) {
	hdrs, ok := req.Header["Anvil-Sess"]
	if !ok || len(hdrs) == 0 {
		return
	}

	return findApiSession(ApiSessionId(hdrs[0]))
}

func (a ApiHandler) parseInitialNumber(s string) (num int, rest string) {
	rest = s
	for i, r := range s {
		if !unicode.IsDigit(r) {
			rest = s[i:]
			break
		}

		num = num*10 + int(r-'0')
	}
	return
}

func (a ApiHandler) serveWindows(rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		a.getWindows(rsp, req)
		return
	} else if req.Method == http.MethodPost {
		a.postWindows(rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) getWindows(rsp http.ResponseWriter, req *http.Request) {
	wins := a.buildWindows()

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(wins)
	flush()
}

func (a ApiHandler) postWindows(rsp http.ResponseWriter, req *http.Request) {
	win := editor.NewWindow(nil)
	if win == nil {
		msg := fmt.Sprintf("Creating new window failed")
		http.Error(rsp, msg, http.StatusInternalServerError)
		return
	}

	log(LogCatgAPI, "ApiHandler.postWindows: created new window with id %d\n", win.Id)
	apiWin := apiWindow{Id: win.Id}

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(apiWin)
	flush()
}

func (a ApiHandler) getEncoder(rsp http.ResponseWriter, req *http.Request) (contentType apiEncoding, enc Encoder, flush func()) {
	typ := req.Header.Get("Accept")
	//"*/*"

	log(LogCatgAPI, "ApiHandler.getEncoder: Accept header is '%s'\n", typ)

	// wins := a.buildWindows()

	if typ == string(encodingTextCsv) {
		contentType = encodingTextCsv
		w := csv.NewWriter(rsp)
		enc = csvutil.NewEncoder(w)
		flush = func() {
			w.Flush()
		}
		return
	}

	contentType = encodingApplicationJson
	jenc := json.NewEncoder(rsp)
	jenc.SetIndent("", "  ")
	enc = jenc
	flush = func() {}

	return
}

func (a ApiHandler) getDecoder(rsp http.ResponseWriter, req *http.Request, csvHeader ...string) (contentType apiEncoding, dec Decoder, err error) {
	typ := req.Header.Get("Content-Type")
	//"*/*"

	log(LogCatgAPI, "ApiHandler.getDecoder: Content-Type header is '%s'\n", typ)

	// wins := a.buildWindows()

	if typ == string(encodingTextCsv) {
		contentType = encodingTextCsv
		r := csv.NewReader(req.Body)
		dec = mylog.Check2(csvutil.NewDecoder(r, csvHeader...))

		return
	}

	contentType = encodingApplicationJson
	dec = json.NewDecoder(req.Body)

	return
}

type Encoder interface {
	Encode(v interface{}) error
}

type Decoder interface {
	Decode(v interface{}) (err error)
}

type apiEncoding string

const (
	encodingTextCsv         apiEncoding = "text/csv"
	encodingApplicationJson             = "application/json"
	encodingTextPlain                   = "text/plain"
)

func (a ApiHandler) buildWindows() apiWindows {
	// Retrieve the editor's list of windows, but run the
	// function in the main goroutine so we don't cause race conditions.
	ch := make(chan []*Window)

	fn := func() {
		ch <- editor.Windows()
	}

	editor.WorkChan() <- basicWork{fn}
	edWins := <-ch

	var wins apiWindows
	for _, w := range edWins {
		wins = append(wins, a.buildWindow(w))
	}
	return wins
}

func (a ApiHandler) buildWindow(w *Window) apiWindow {
	finder := NewFileFinder(w)
	file := mylog.Check2(finder.WindowFile())

	return apiWindow{
		Id:         w.Id,
		GlobalPath: w.file,
		Path:       file,
	}
}

type apiWindows []apiWindow

type apiWindow struct {
	Id         int
	GlobalPath string
	Path       string
}

func (a ApiHandler) buildWindowBody(w *Window) apiWindowBody {
	return apiWindowBody{
		Len: w.Body.Len(),
	}
}

type apiWindowBody struct {
	Len int
}

func (a ApiHandler) serveWindowBody(winId int, rsp http.ResponseWriter, req *http.Request, subpath string) {
	switch subpath {
	case "/body/info":
		a.serveWindowBodyInfo(winId, rsp, req)
	case "/body/cursors":
		a.serveWindowBodyCursors(winId, rsp, req)
	default:
		a.serveWindowBodyContent(winId, rsp, req)
	}
}

func (a ApiHandler) serveWindowBodyInfo(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	b := a.buildWindowBody(win)

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(b)
	flush()
}

func (a ApiHandler) serveWindowBodyCursors(winId int, rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		log(LogCatgAPI, "ApiHandler.serveWindowBodyCursors: request to get cursors\n")
		a.getWindowBodyCursors(winId, rsp, req)
		return
	} else if req.Method == http.MethodPut {
		log(LogCatgAPI, "ApiHandler.serveWindowBodyCursors: request to put cursors\n")
		a.putWindowBodyCursors(winId, rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) getWindowBodyCursors(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []int)
	fn := func() {
		s := make([]int, len(win.Body.CursorIndices))
		copy(s, win.Body.CursorIndices)
		ch <- s
	}

	editor.WorkChan() <- basicWork{fn}
	cursors := <-ch

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(cursors)
	flush()
}

func (a ApiHandler) putWindowBodyCursors(winId int, rsp http.ResponseWriter, req *http.Request) {
	// We need to check the encoding of the request body that was sent usign the header, and then
	// decode it using the right decoder (CSV or JSON).

	var cursors []int

	_, dec := mylog.Check3(a.getDecoder(rsp, req, "cursor_index"))
	mylog.Check(dec.Decode(&cursors))

	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []int)
	fn := func() {
		cursors := <-ch
		win.Body.SetCursorIndices(cursors)
		return
	}

	editor.WorkChan() <- basicWork{fn}
	ch <- cursors
}

func (a ApiHandler) serveWindowBodyContent(winId int, rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		log(LogCatgAPI, "ApiHandler.serveWindowBody: request to get content\n")
		a.getWindowBodyContent(winId, rsp, req)
		return
	} else if req.Method == http.MethodPut {
		log(LogCatgAPI, "ApiHandler.serveWindowBody: request to put content\n")
		a.putWindowBodyContent(winId, rsp, req)
		return
	} else if req.Method == http.MethodPost {
		log(LogCatgAPI, "ApiHandler.serveWindowBody: request to post content\n")
		a.postWindowBodyContent(winId, rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) getWindowBodyContent(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []byte)
	fn := func() {
		ch <- win.Body.Bytes()
	}

	editor.WorkChan() <- basicWork{fn}
	content := <-ch

	rsp.Header().Add("Content-Type", encodingTextPlain)
	rsp.Write(content)
}

func (a ApiHandler) putWindowBodyContent(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []byte)
	fn := func() {
		data, ok := <-ch
		if !ok {
			return
		}
		ci := win.Body.blockEditable.firstCursorIndex()
		tl := win.Body.TopLeftIndex
		win.Body.SetText(data)
		win.SetTag()
		win.Body.AddOpForNextLayout(func(gtx layout.Context) {
			win.Body.moveCursorTo(gtx, seek{seekType: seekToRunePos, runePos: ci}, dontSelectText)
			win.Body.TopLeftIndex = tl
		})
	}

	editor.WorkChan() <- basicWork{fn}
	data := mylog.Check2(ioutil.ReadAll(req.Body))

	ch <- data
}

func (a ApiHandler) postWindowBodyContent(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []byte)
	fn := func() {
		data, ok := <-ch
		if !ok {
			return
		}

		win.Body.Append(data)
		/*
			ci := win.Body.blockEditable.firstCursorIndex()
			tl := win.Body.TopLeftIndex
			win.Body.SetText(data)
		*/
		win.SetTag()
		/*win.Body.AddOpForNextLayout(func(gtx layout.Context) {
			win.Body.moveCursorTo(gtx, seek{seekType: seekToRunePos, runePos: ci}, dontSelectText)
			win.Body.TopLeftIndex = tl
		})
		*/
	}

	editor.WorkChan() <- basicWork{fn}
	data := mylog.Check2(ioutil.ReadAll(req.Body))

	ch <- data
}

func (a ApiHandler) serveWindowSelections(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []*selection)
	fn := func() {
		ch <- win.Body.copySelections()
	}

	editor.WorkChan() <- basicWork{fn}
	sels := <-ch

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(a.buildSelections(sels))
	flush()
}

func (a ApiHandler) FindWindowForId(winId int) *Window {
	ch := make(chan *Window)

	fn := func() {
		ch <- editor.FindWindowForId(winId)
	}

	editor.WorkChan() <- basicWork{fn}
	return <-ch
}

type apiSelection struct {
	Start, End, Len int
}

func (a ApiHandler) buildSelections(sels []*selection) []apiSelection {
	rc := make([]apiSelection, len(sels))
	for i, s := range sels {
		rc[i] = apiSelection{s.start, s.end, s.end - s.start}
	}

	return rc
}

func (a ApiHandler) serveWindowInfo(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	aw := a.buildWindow(win)

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(aw)
	flush()
}

func (a ApiHandler) serveWindowTag(winId int, rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		a.getWindowTag(winId, rsp, req)
		return
	} else if req.Method == http.MethodPut {
		a.putWindowTag(winId, rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) getWindowTag(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []byte)
	fn := func() {
		ch <- win.Tag.Bytes()
	}

	editor.WorkChan() <- basicWork{fn}
	content := <-ch

	rsp.Header().Add("Content-Type", encodingTextPlain)
	rsp.Write(content)
}

func (a ApiHandler) putWindowTag(winId int, rsp http.ResponseWriter, req *http.Request) {
	win := a.FindWindowForId(winId)

	if win == nil {
		msg := fmt.Sprintf("No window with id %d", winId)
		http.Error(rsp, msg, http.StatusNotFound)
		return
	}

	ch := make(chan []byte)
	fn := func() {
		data, ok := <-ch
		if !ok {
			return
		}

		file := ""
		edArea := ""
		s := string(data)
		parts, _, e := (calculateTagParts(s))
		if e == nil {
			file = s[parts.path[0]:parts.path[1]]
			edArea = s[parts.editorArea[0]:parts.editorArea[1]]
		}

		win.file = file
		win.fileType = typeFile
		win.initialTagUserArea = ""
		win.customEdCommands = edArea
		log(LogCatgAPI, "APIHandler: setting window %d tag to '%s'\n", winId, data)
		win.Tag.SetText(data)
		return
	}

	editor.WorkChan() <- basicWork{fn}
	data := mylog.Check2(ioutil.ReadAll(req.Body))

	ch <- data
}

func (a ApiHandler) serveJobs(rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		a.getJobs(rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) getJobs(rsp http.ResponseWriter, req *http.Request) {
	jobs := a.buildJobs()

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(jobs)
	flush()
}

func (a ApiHandler) buildJobs() apiJobs {
	// Retrieve the editor's list of windows, but run the
	// function in the main goroutine so we don't cause race conditions.
	ch := make(chan []Job)

	fn := func() {
		ch <- editor.Jobs()
	}

	editor.WorkChan() <- basicWork{fn}
	edJobs := <-ch

	var jobs apiJobs
	for _, j := range edJobs {
		jobs = append(jobs, a.buildJob(j))
	}
	return jobs
}

func (a ApiHandler) buildJob(j Job) apiJob {
	return apiJob{
		Name: j.Name(),
	}
}

type apiJobs []apiJob

type apiJob struct {
	Name string
}

func (a ApiHandler) serveNotifs(sess *ApiSession, rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		a.getNotifs(sess, rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) getNotifs(sess *ApiSession, rsp http.ResponseWriter, req *http.Request) {
	notifs := apiGetAndClearNotifications(sess.Id())

	contentType, enc, flush := a.getEncoder(rsp, req)

	rsp.Header().Add("Content-Type", string(contentType))
	enc.Encode(notifs)
	flush()
}

func (a ApiHandler) serveCmds(sess *ApiSession, rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		log(LogCatgAPI, "ApiHandler.serveCmds: request to post content\n")
		a.registerUserDefinedCommands(sess, rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) registerUserDefinedCommands(sess *ApiSession, rsp http.ResponseWriter, req *http.Request) {
	var cmds []string

	_, dec := mylog.Check3(a.getDecoder(rsp, req, "cmd"))
	mylog.Check(dec.Decode(&cmds))

	for _, c := range cmds {
		log(LogCatgAPI, "ApiHandler.registerUserDefinedCommands: registering command %s\n", c)
		sess.addUserDefinedCommand(c)
	}
	updateApiSession(sess)
}

func (a ApiHandler) serveExecute(sess *ApiSession, rsp http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		log(LogCatgAPI, "ApiHandler.serveCmds: request to execute command\n")
		a.executeOnEditor(sess, rsp, req)
		return
	}

	msg := fmt.Sprintf("Method %s is not supported for %s", req.Method, req.URL.Path)
	http.Error(rsp, msg, http.StatusBadRequest)
}

func (a ApiHandler) executeOnEditor(sess *ApiSession, rsp http.ResponseWriter, req *http.Request) {
	var cmd apiExecuteReq

	_, dec := mylog.Check3(a.getDecoder(rsp, req, "cmd", "args"))
	mylog.Check(dec.Decode(&cmd))

	editor.Execute(cmd.Cmd, cmd.Args)
}

type apiExecuteReq struct {
	Cmd  string
	Args []string
}

type notifs []ApiNotification

type ApiSessionId string

type ApiSessionStore struct {
	sessions map[ApiSessionId]*ApiSession
	lock     sync.Mutex
	max      int
}

func NewApiSessionStore(maxSessions int) ApiSessionStore {
	return ApiSessionStore{
		sessions: map[ApiSessionId]*ApiSession{},
		max:      maxSessions,
	}
}

func (s *ApiSessionStore) Add(sess *ApiSession) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.sessions) >= s.max {
		return fmt.Errorf("There are too many API sessions to make a new one. The max of %d has been reached", s.max)
	}

	s.sessions[sess.id] = sess
	return nil
}

func (s *ApiSessionStore) Update(sess *ApiSession) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.sessions[sess.id]; !ok {
		return
	}
	s.sessions[sess.id] = sess
}

func (s *ApiSessionStore) Find(id ApiSessionId) (sess ApiSession, ok bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	ptr, ok := s.sessions[id]
	if ok {
		sess = *ptr
	}

	return
}

func (s *ApiSessionStore) Del(id ApiSessionId) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.sessions, id)
}

func (s *ApiSessionStore) Len() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.sessions)
}

func (s *ApiSessionStore) All() []*ApiSession {
	s.lock.Lock()
	defer s.lock.Unlock()

	r := make([]*ApiSession, 0, len(s.sessions))
	for _, v := range s.sessions {
		r = append(r, v)
	}
	return r
}

func (s *ApiSessionStore) AddNotificationToAll(n ApiNotification) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, sess := range s.sessions {
		sess.AddNotification(n)
	}
}

func (s *ApiSessionStore) GetAndClearNotifications(id ApiSessionId) []ApiNotification {
	s.lock.Lock()
	defer s.lock.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return []ApiNotification{}
	}

	r := sess.pendingNotifications
	sess.pendingNotifications = nil
	return r
}

func (s *ApiSessionStore) HandleCommand(winId int, cmd string, args []string) (handled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	log(LogCatgAPI, "ApiSessionStore.HandleCommand: called\n")
	for _, sess := range s.sessions {
		log(LogCatgAPI, "ApiSessionStore.HandleCommand: checking session. Has %d commands\n", len(sess.userDefinedCommands))
		for _, scmd := range sess.userDefinedCommands {
			log(LogCatgAPI, "ApiSessionStore.HandleCommand: checking command %s vs %s\n", cmd, scmd)
			if scmd == cmd {
				n := newCommandApiNotification(winId, cmd, args)
				sess.AddNotification(n)

				handled = true
				// Check other sessions to see if they are interested as well.
				break
			}
		}
	}
	return
}

func newCommandApiNotification(winId int, cmd string, args []string) ApiNotification {
	c := make([]string, len(args)+1)
	c[0] = cmd
	copy(c[1:], args)

	return ApiNotification{
		WinId: winId,
		Op:    ApiNotificationOpExec,
		Cmd:   c,
	}
}

var apiSessions = NewApiSessionStore(maxApiSessions)

const (
	maxApiSessions                = 100
	maxApiNotificationsPerSession = 100
)

type ApiSession struct {
	id                   ApiSessionId
	pendingNotifications []ApiNotification
	cmd                  string
	userDefinedCommands  []string
}

func createApiSession(cmd string) (sess *ApiSession, err error) {
	buf := make([]byte, 200)

	_ = mylog.Check2(rand.Read(buf))

	sess = &ApiSession{
		id:  ApiSessionId(base64.StdEncoding.EncodeToString(buf)),
		cmd: cmd,
	}

	apiSessions.Add(sess)

	return
}

func updateApiSession(sess *ApiSession) {
	apiSessions.Update(sess)
}

func findApiSession(id ApiSessionId) (sess ApiSession, ok bool) {
	return apiSessions.Find(id)
}

func deleteApiSession(id ApiSessionId) {
	apiSessions.Del(id)
}

func getApiSessions() []*ApiSession {
	return apiSessions.All()
}

func addApiNotificationToAllSessions(n ApiNotification) {
	apiSessions.AddNotificationToAll(n)
}

func apiGetAndClearNotifications(id ApiSessionId) []ApiNotification {
	return apiSessions.GetAndClearNotifications(id)
}

func apiHandleCommand(winId int, cmd string, args []string) (handled bool) {
	return apiSessions.HandleCommand(winId, cmd, args)
}

func (s *ApiSession) Id() ApiSessionId {
	return s.id
}

func (s *ApiSession) Cmd() string {
	return s.cmd
}

func (s *ApiSession) AddNotification(n ApiNotification) {
	if len(s.pendingNotifications) >= maxApiNotificationsPerSession {
		return
	}
	s.pendingNotifications = append(s.pendingNotifications, n)
}

func (a *ApiSession) addUserDefinedCommand(s string) {
	t := a.textBeforeFirstSpace(s)
	log(LogCatgAPI, "ApiSession.addUserDefinedCommand: cmd: '%s' cleaned: '%s'\n", s, t)
	if t == "" {
		return
	}
	a.userDefinedCommands = append(a.userDefinedCommands, t)
}

func (a *ApiSession) textBeforeFirstSpace(s string) string {
	rns := []rune(s)
	var buf bytes.Buffer
	for _, r := range rns {
		if unicode.IsSpace(r) {
			break
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

type ApiNotification struct {
	WinId  int
	Op     ApiNotificationOp
	Offset int
	Len    int
	Cmd    []string
}

type ApiNotificationOp int

const (
	ApiNotificationOpInsert = iota
	ApiNotificationOpDelete
	ApiNotificationOpExec
)
