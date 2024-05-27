package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ddkwork/golibrary/mylog"
	"golang.org/x/crypto/ssh"
)

func isWindowsPath(path string) bool {
	return len(path) >= 3 &&
		((path[0] >= 'A' && path[0] < 'Z') || (path[0] >= 'a' && path[0] <= 'z')) &&
		path[1] == ':' && path[2] == '\\'
}

type FileFinder struct {
	win *Window
}

func NewFileFinder(w *Window) *FileFinder {
	return &FileFinder{
		win: w,
	}
}

// Find looks for the file or directory `fpath` to make it a complete path. If path exists locally then `existsLocal` is
// set to true. If it is a remote path, it is not checked so as to not block the caller for the time needed to open an ssh connection
// for the first time.
func (f FileFinder) Find(fpath string) (fullpath *GlobalPath, existsLocal bool, err error) {
	var lfs localFs

	var gpath *GlobalPath
	gpath = mylog.Check2(NewGlobalPath(fpath, GlobalPathUnknown))

	winpath := mylog.Check2(f.winFile())

	log(LogCatgFS, "FileFinder.Find: window file is '%v'\n", winpath)

	if gpath.IsRemote() {
		// If it's a remote path then it's already complete. For example,
		// the relative ssh path host:dir/file is automatically treated as relative
		// to the home directory by the ssh server, so there is no need to complete it.
		fullpath = gpath
		return
	}

	if gpath.IsAbsolute() || (winpath.IsRemote() && path.IsAbs(fpath)) {
		log(LogCatgFS, "FileFinder.Find: path is absolute\n")
		existsLocal = mylog.Check2(lfs.fileExists(fpath))

		if existsLocal {
			log(LogCatgFS, "FileFinder.Find: path exists locally\n")
			fullpath = mylog.Check2(NewGlobalPath(fpath, GlobalPathUnknown))
			return
		}

		if winpath.IsRemote() && !gpath.IsRemote() {
			gpath = gpath.GlobalizeRelativeTo(winpath)
			log(LogCatgFS, "FileFinder.Find: window path is remote and requested path is not; using remote info from window path. Path is now: %s\n", gpath)
		}
		// window path is not remote but the path requested is absolute and doesn't exist locally.
		fullpath = gpath
		return
	}

	// path is relative
	fullpath = gpath.MakeAbsoluteRelativeTo(winpath)
	log(LogCatgFS, "FileFinder.Find: path is relative\n")
	log(LogCatgFS, "FileFinder.Find: full path combined with window path is %s\n", fullpath)
	if fullpath.IsRemote() {
		log(LogCatgFS, "FileFinder.Find: path combined with window path is remote\n")
		existsLocal = true
		return
	}

	existsLocal = mylog.Check2(lfs.fileExists(fullpath.String()))
	log(LogCatgFS, "FileFinder.Find: path combined with window path exists: %v\n", existsLocal)

	return
}

func (f FileFinder) winFile() (path *GlobalPath, err error) {
	var lfs localFs
	rfs := NewSshFs(sshOptsFromSettings())

	path = mylog.Check2(f.winFileNoCheck())

	if path.dirState == GlobalPathUnknown {
		var isDir bool
		if path.IsRemote() {
			if f.win != nil && f.win.fileType != typeUnknown {
				// This saves needing to use the ssh connection to tell the filetype
				isDir = f.win.fileType == typeDir
			} else {
				isDir = mylog.Check2(rfs.isDir(path.String()))
			}
		} else {
			isDir = mylog.Check2(lfs.isDir(path.String()))
		}

		if isDir {
			path.SetDirState(GlobalPathIsDir)
		} else {
			path.SetDirState(GlobalPathIsFile)
		}
	}

	return
}

func (f FileFinder) winFileNoCheck() (path *GlobalPath, err error) {
	state := GlobalPathUnknown
	p := "."
	if f.win != nil {
		p = f.win.file
		if strings.HasSuffix(p, "+Errors") {
			p = p[:len(p)-7]
			state = GlobalPathIsDir
		}
		if f.win.fileType == typeDir {
			state = GlobalPathIsDir
		}
	} else {
		state = GlobalPathIsDir
	}

	path = mylog.Check2(NewGlobalPath(p, state))
	return
}

func (f FileFinder) WindowDir() (path string, err error) {
	winpath := mylog.Check2(f.winFileNoCheck())
	path = winpath.Dir().String()
	return
}

func (f FileFinder) WindowFile() (path string, err error) {
	winpath := mylog.Check2(f.winFileNoCheck())
	path = winpath.Path()
	return
}

type FileLoader struct{}

func (l *FileLoader) Load(path string) (contents []byte, filenames []string, err error) {
	sfs := mylog.Check2(GetFs(path))

	isDir := mylog.Check2(sfs.isDir(path))

	if isDir {
		filenames = mylog.Check2(sfs.filenamesInDir(path))
	} else {
		contents = mylog.Check2(sfs.loadFile(path))
	}
	return
}

func (l *FileLoader) LoadAsync(path string) (load *DataLoad, err error) {
	sfs := mylog.Check2(GetFs(path))

	load = NewDataLoad()
	mylog.Check(sfs.contentsAsync(path, load.Filenames, load.Contents, load.Errs, load.Kill))

	return
}

type DataLoad struct {
	Contents  chan []byte
	Filenames chan []string
	Errs      chan error // Will only contain one error
	Kill      chan struct{}
}

func NewDataLoad() *DataLoad {
	return &DataLoad{
		Errs:      make(chan error),
		Kill:      make(chan struct{}, 1),
		Contents:  make(chan []byte),
		Filenames: make(chan []string),
	}
}

// Save a local or remote file .
func (l *FileLoader) Save(path string, contents []byte) (err error) {
	sfs := mylog.Check2(GetFs(path))
	mylog.Check(sfs.saveFile(path, contents))

	return
}

func (l *FileLoader) SaveAsync(path string, contents []byte) (save *DataSave, err error) {
	sfs := mylog.Check2(GetFs(path))

	save = NewDataSave()
	mylog.Check(sfs.saveFileAsync(path, contents, save.Errs, save.Kill))

	return
}

type DataSave struct {
	Errs chan error // Will only contain one error
	Kill chan struct{}
}

func NewDataSave() *DataSave {
	return &DataSave{
		Errs: make(chan error),
		Kill: make(chan struct{}, 1),
	}
}

func GetFs(path string) (sfs simpleFs, err error) {
	// Local file or dir?
	isRemote := mylog.Check2(isRemoteFilenameOrDir(path))

	if isRemote {
		log(LogCatgFS, "FileLoader: using ssh\n")
		r := NewSshFs(sshOptsFromSettings())
		sfs = r
	} else {
		log(LogCatgFS, "FileLoader: using local filesystem\n")
		var l localFs
		sfs = l
	}
	return
}

func sshOptsFromSettings() sshFsOpts {
	return sshFsOpts{
		shell:      settings.Ssh.Shell,
		closeStdin: settings.Ssh.CloseStdin,
	}
}

type simpleFs interface {
	fileExists(path string) (ok bool, err error)
	isDir(path string) (ok bool, err error)
	isDirAsync(path string, kill chan struct{}) (ok bool, err error)
	loadFile(path string) (contents []byte, err error)
	loadFileAsync(path string, contents chan []byte, errs chan error, kill chan struct{}) (err error)
	saveFile(path string, contents []byte) (err error)
	saveFileAsync(path string, contents []byte, errs chan error, kill chan struct{}) (err error)
	filenamesInDir(path string) (names []string, err error)
	filenamesInDirAsync(path string, names chan []string, errs chan error, kill chan struct{}) (err error)
	exec(dir, cmd, arg string) (output []byte, err error)
	// execAsync(dir, cmd, arg string, stdin []byte, contents chan []byte, errs chan error, kill chan struct{}) (err error)
	execAsync(execCtx) (err error)
	contentsAsync(path string, names chan []string, contents chan []byte, errs chan error, kill chan struct{}) (err error)
}

type execCtx struct {
	dir         string
	cmd         string
	arg         string
	stdin       []byte
	contents    chan []byte
	errs        chan error
	kill        chan struct{}
	extraEnv    []string
	done        chan struct{}
	shellString string
}

func (c execCtx) fullEnv() []string {
	return append(os.Environ(), c.extraEnv...)
}

func (c execCtx) extraEnvNamesAndValues() (names, values []string, err error) {
	for _, e := range c.extraEnv {
		parts := strings.Split(e, "=")
		if len(parts) != 2 {
			mylog.Check(fmt.Errorf("Invalid environment variable set: %s\n", e))
			return
		}
		names = append(names, parts[0])
		values = append(values, parts[1])
	}
	return
}

type localFs struct{}

func (f localFs) fileExists(path string) (ok bool, err error) {
	return fileExists(path)
}

func (f localFs) isDir(path string) (ok bool, err error) {
	return isDir(path)
}

func (f localFs) isDirAsync(path string, kill chan struct{}) (ok bool, err error) {
	return isDir(path)
}

func (f localFs) loadFile(path string) (contents []byte, err error) {
	return ioutil.ReadFile(path)
}

func (f localFs) loadFileAsync(path string, contents chan []byte, errs chan error, kill chan struct{}) (err error) {
	file := mylog.Check2(os.Open(path))

	go func() {
		copyBlocks(file, contents, 1024*1024, errs, kill)
		close(errs)
	}()
	return
}

func copyBlocks(source io.Reader, dest chan []byte, blocksize int, errs chan error, kill chan struct{}) {
	defer close(dest)

	count := 0
	updateBlockSize := func() {
		if blocksize >= 1048576 {
			return
		}

		if count < 50 {
			count++
			return
		}

		blocksize = 1048576
	}

	for {
		block := make([]byte, blocksize)
		if kill != nil {
			select {
			case <-kill:
				return
			default:
			}
		}

		n, e := source.Read(block)
		if mylog.CheckEof(e) {
			break
		}
		// errs might already be closed, hence we send in a select statement

		if n == 0 {
			continue
		}

		b := block
		if n < len(block) {
			b = block[:n]
		}
		dest <- b

		updateBlockSize()
	}
}

func (f localFs) saveFile(path string, contents []byte) (err error) {
	return ioutil.WriteFile(path, contents, 0664)
}

func (f localFs) saveFileAsync(path string, contents []byte, errs chan error, kill chan struct{}) (err error) {
	go func() {
		mylog.Check(f.saveFile(path, contents))

		close(errs)
	}()
	return nil
}

func (f localFs) filenamesInDir(path string) (names []string, err error) {
	return filenamesInDir(path)
}

func (f localFs) filenamesInDirAsync(path string, names chan []string, errs chan error, kill chan struct{}) (err error) {
	// TODO: make this more asynchronous for huge directories
	go func() {
		lnames := mylog.Check2(filenamesInDir(path))

		names <- lnames
		close(names)
		close(errs)
	}()
	return
}

func (f localFs) contentsAsync(path string, names chan []string, contents chan []byte, errs chan error, kill chan struct{}) (err error) {
	isDir := mylog.Check2(f.isDir(path))

	if isDir {
		mylog.Check(f.filenamesInDirAsync(path, names, errs, kill))
	} else {
		mylog.Check(f.loadFileAsync(path, contents, errs, kill))
	}

	return
}

func (f localFs) exec(dir, command, arg string) (output []byte, err error) {
	var out bytes.Buffer
	args := fmt.Sprintf("%s %s", command, arg)
	cmd := exec.Command("bash", "-c", args)
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Dir = dir
	mylog.Check(cmd.Run())
	output = out.Bytes()
	return
}

func (f localFs) execAsync(c execCtx) (err error) {
	cmd, _, _, closed, apiSess := mylog.Check6(f.setupForAsyncExec(c))
	mylog.Check(cmd.Start())

	go func() {
		_, ok := <-c.kill
		if ok {
			mylog.Check(KillProcess(cmd.Process))
		}
	}()

	go func() {
		// It seems that calling Wait too soon on a Command causes
		// an error like "read |0: file already closed". It seems to be recommended
		// to only call Wait after reading is finished.
		<-closed
		time.Sleep(200 * time.Millisecond)
		mylog.Check(cmd.Wait())
		log(LogCatgFS, "localFs.execAsync: wait error: %v\n", err)

		close(c.errs)
		/* Fix leak in goroutines */
		close(c.kill)
		if c.done != nil {
			close(c.done)
		}
		if apiSess != nil {
			deleteApiSession(apiSess.Id())
		}
	}()

	return
}

func (f localFs) setupForAsyncExec(c execCtx) (cmd *exec.Cmd, stdout, stderr io.ReadCloser, closed chan struct{}, apiSess *ApiSession, err error) {
	args := fmt.Sprintf("%s %s", c.cmd, c.arg)
	if runtime.GOOS == "windows" {
		cmd = WindowsCmd(args)
	} else {
		cmd = exec.Command("bash", "-c", args)
	}

	if c.stdin != nil {
		cmd.Stdin = bytes.NewBuffer(c.stdin)
	}

	stdout = mylog.Check2(cmd.StdoutPipe())

	stderr = mylog.Check2(cmd.StderrPipe())

	cmd.Dir = c.dir

	if c.extraEnv != nil {
		cmd.Env = c.fullEnv()
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("ANVIL_API_PORT=%d", LocalAPIPort()))

	apiSess = mylog.Check2(createApiSession(args))

	cmd.Env = append(cmd.Env, fmt.Sprintf("ANVIL_API_SESS=%s", apiSess.Id()))

	c3, closed := signalWhenComplete(c.contents)
	c1, c2 := mergeContentsInto(c3)

	go copyBlocks(stdout, c1, 1024*1024, c.errs, nil)
	go copyBlocks(stderr, c2, 1024*1024, c.errs, nil)

	return
}

func forkKill(kill chan struct{}) (kill2, kill3 chan struct{}) {
	kill2 = make(chan struct{})
	kill3 = make(chan struct{})

	go func() {
		for i := range kill {
			kill2 <- i
			kill3 <- i
		}
		close(kill2)
		close(kill3)
	}()
	return
}

func mergeContentsInto(dest chan []byte) (c1, c2 chan []byte) {
	c1 = make(chan []byte)
	c2 = make(chan []byte)

	go func() {
		var eofs [2]bool
		for !(eofs[0] && eofs[1]) {
			select {
			case b, ok := <-c1:
				if !ok {
					eofs[0] = true
					c1 = nil
					continue
				}
				dest <- b
			case b, ok := <-c2:
				if !ok {
					eofs[1] = true
					c2 = nil
					continue
				}
				dest <- b
			}
		}
		close(dest)
	}()
	return
}

// signalWhenComplete copies src to dest, and closes sig when src is closed
func signalWhenComplete(dest chan []byte) (src chan []byte, sig chan struct{}) {
	src = make(chan []byte)
	sig = make(chan struct{})

	go func() {
		for x := range src {
			dest <- x
		}
		close(sig)
		close(dest)
	}()

	return
}

func isRemoteFilenameOrDir(path string) (b bool, err error) {
	b = mylog.Check2(fileExists(path))
	if b && err == nil {
		return false, nil
	}

	var gp *GlobalPath
	gp = mylog.Check2(NewGlobalPath(path, GlobalPathUnknown))

	b = gp.IsRemote()
	return
}

type sshFs struct {
	shell      string
	closeStdin bool
}

func NewSshFs(opts sshFsOpts) *sshFs {
	return &sshFs{
		shell:      opts.shell,
		closeStdin: opts.closeStdin,
	}
}

type sshFsOpts struct {
	shell      string
	closeStdin bool
}

func (f *sshFs) getShell() string {
	if f.shell == "" {
		return "sh"
	}
	return f.shell
}

func (f *sshFs) fileExists(path string) (ok bool, err error) {
	file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, nil))

	defer session.Close()

	log(LogCatgFS, "sshFs.fileExists: checking for %s\n", path)

	cmd := fmt.Sprintf("%s -c 'if [ -e \"%s\" ]; then echo yes; else echo no; fi'", f.getShell(), file)
	log(LogCatgFS, "sshFs.fileExists: running command: %s\n", cmd)
	b := mylog.Check2(session.Output(cmd))

	log(LogCatgFS, "sshFs.fileExists: got output %s \n", string(b))

	s := string(b)
	if s == "yes\n" {
		ok = true
	}
	log(LogCatgFS, "sshFs.fileExists: returning %v,%v\n", ok, err)

	return
}

func (f *sshFs) isDirAsync(path string, kill chan struct{}) (ok bool, err error) {
	file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, kill))

	defer session.Close()

	cmd := fmt.Sprintf("%s -c 'if [ -d \"%s\" ]; then echo yes; else echo no; fi'", f.getShell(), file)
	b := mylog.Check2(session.Output(cmd))

	s := string(b)
	if s == "yes\n" {
		ok = true
	}

	return
}

func (f *sshFs) isDir(path string) (ok bool, err error) {
	return f.isDirAsync(path, nil)
}

func (f *sshFs) splitFilenameAndMakeSession(path string, kill chan struct{}) (file string, session *ssh.Session, client *SshClient, err error) {
	client, file = mylog.Check3(f.splitFilenameAndDial(path, kill))

	session = mylog.Check2(client.NewSession())
	return
}

func (f *sshFs) splitFilenameAndDial(path string, kill chan struct{}) (client *SshClient, file string, err error) {
	gpath := mylog.Check2(NewGlobalPath(path, GlobalPathUnknown))

	log(LogCatgFS, "sshFs: split path %s into %#v\n", path, gpath)
	file = gpath.Path()

	endpt := SshEndpt{
		Dest: SshHop{
			User: gpath.User(),
			Host: gpath.Host(),
			Port: gpath.Port(),
		},
		Proxy: SshHop{
			User: gpath.ProxyUser(),
			Host: gpath.ProxyHost(),
			Port: gpath.ProxyPort(),
		},
	}
	client = mylog.Check2(f.dial(endpt, kill))
	return
}

func (f *sshFs) dial(endpt SshEndpt, kill chan struct{}) (client *SshClient, err error) {
	client = mylog.Check2(sshClientCache.Get(endpt, kill))
	log(LogCatgFS, "sshFs: retrieved ssh client from cache for %s. Error (if any)=%v\n", endpt, err)
	return
}

func (f *sshFs) loadFile(path string) (contents []byte, err error) {
	file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, nil))

	defer session.Close()

	// sh -c 'if [ -d /tmp ]; then echo yes; else echo no; fi'
	cmd := fmt.Sprintf("%s -c 'cat \"%s\"'", f.getShell(), file)
	contents = mylog.Check2(session.Output(cmd))

	return
}

func (f *sshFs) loadFileAsync(path string, contents chan []byte, errs chan error, kill chan struct{}) (err error) {
	go func() {
		file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, kill))

		cmd := fmt.Sprintf("%s -c 'cat \"%s\"'", f.getShell(), file)

		stdout := mylog.Check2(session.StdoutPipe())

		go func() {
			copyBlocks(stdout, contents, 4096, errs, kill)
			session.Close()
			close(errs)
		}()
		mylog.Check(session.Start(cmd))
	}()

	return nil
}

func (f *sshFs) saveFile(path string, contents []byte) (err error) {
	file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, nil))

	defer session.Close()

	// sh -c 'if [ -d /tmp ]; then echo yes; else echo no; fi'
	cmd := fmt.Sprintf("%s -c 'cat > \"%s\"'", f.getShell(), file)
	pipe := mylog.Check2(session.StdinPipe())
	mylog.Check(session.Start(cmd))

	_ = mylog.Check2(pipe.Write(contents))

	pipe.Close()
	mylog.Check(session.Wait())

	return
}

func (f sshFs) saveFileAsync(path string, contents []byte, errs chan error, kill chan struct{}) (err error) {
	// return fmt.Errorf("Not implemented yet")
	go func() {
		file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, kill))

		cmd := fmt.Sprintf("%s -c 'cat > \"%s\"'", f.getShell(), file)
		log(LogCatgFS, "sshFs.saveFileAsync: running command: %s\n", cmd)

		pipe := mylog.Check2(session.StdinPipe())
		mylog.Check(session.Start(cmd))

		go func() {
			_, ok := <-kill
			if !ok {
				return
			}
			session.Close()
			mylog.Check(session.Wait())

			close(errs)
		}()

		_ = mylog.Check2(pipe.Write(contents))

		pipe.Close()
		mylog.Check(session.Wait())

		close(errs)
	}()

	return nil
}

func (f *sshFs) filenamesInDir(path string) (names []string, err error) {
	file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, nil))

	defer session.Close()

	cmd := fmt.Sprintf("%s -c 'ls -Ap \"%s\" | cat'", f.getShell(), file)
	b := mylog.Check2(session.Output(cmd))

	names = strings.Split(string(b), "\n")

	return
}

func (f *sshFs) filenamesInDirAsync(path string, names chan []string, errs chan error, kill chan struct{}) (err error) {
	file, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, kill))

	// TODO: make this more asynchronous for huge directories
	go func() {
		cmd := fmt.Sprintf("%s -c 'ls -Ap \"%s\" | cat'", f.getShell(), file)
		b := mylog.Check2(session.Output(cmd))

		lnames := strings.Split(string(b), "\n")
		names <- lnames
		session.Close()
		close(names)
		close(errs)
	}()

	return
}

func (f *sshFs) contentsAsync(path string, names chan []string, contents chan []byte, errs chan error, kill chan struct{}) (err error) {
	go func() {
		isDir := mylog.Check2(f.isDirAsync(path, kill))

		if isDir {
			mylog.Check(f.filenamesInDirAsync(path, names, errs, kill))
		} else {
			mylog.Check(f.loadFileAsync(path, contents, errs, kill))
		}
	}()

	return nil
}

func (f sshFs) exec(path, command, arg string) (output []byte, err error) {
	dir, session, _ := mylog.Check4(f.splitFilenameAndMakeSession(path, nil))

	defer session.Close()

	cmd := fmt.Sprintf("%s -c 'cd \"%s\" && %s %s'", f.getShell(), dir, command, arg)
	log(LogCatgFS, "sshFs.exec: running command: %s\n", cmd)
	output = mylog.Check2(session.Output(cmd))
	return
}

func (f sshFs) execAsync(c execCtx) (err error) {
	go func() {
		session, cmd, apiSess, ok := f.setupForAsyncExec(c)
		if !ok {
			return
		}
		mylog.Check(session.Start(cmd))

		go func() {
			<-c.kill
			log(LogCatgFS, "sshFs.exec: kill received. Closing session\n")
			// See https://github.com/golang/go/issues/16597
			session.Signal(ssh.SIGKILL)
			session.Close()
			log(LogCatgFS, "sshFs.exec: kill: waiting for status\n")
			mylog.Check(session.Wait())
			log(LogCatgFS, "sshFs.exec: kill: wait done\n")

			log(LogCatgFS, "sshFs.exec: kill: killing finished\n")
			close(c.errs)
		}()

		go func() {
			mylog.Check(session.Wait())
			log(LogCatgFS, "sshFs.exec: wait error: %v\n", err)

			close(c.errs)
			if c.done != nil {
				close(c.done)
			}
			if apiSess != nil {
				deleteApiSession(apiSess.Id())
			}
		}()
	}()

	return
}

func (f sshFs) setupForAsyncExec(c execCtx) (session *ssh.Session, cmd string, apiSess *ApiSession, ok bool) {
	dir, session, client := mylog.Check4(f.splitFilenameAndMakeSession(c.dir, c.kill))

	extra := ""
	if c.stdin == nil && f.closeStdin {
		extra = " 0<&-" // shell command to close stdin
	}

	cmd = buildShellString(c, f.getShell(), dir, extra)
	log(LogCatgFS, "sshFs.exec: running command: %s\n", cmd)

	if c.stdin != nil {
		session.Stdin = bytes.NewBuffer(c.stdin)
	}

	stdout := mylog.Check2(session.StdoutPipe())

	stderr := mylog.Check2(session.StderrPipe())
	mylog.Check(

		// We start an API server on the listener if one is not started.
		// But each execution of a command gets a new session id
		f.maybeServeAPIOverSshClient(client))

	apiSess = mylog.Check2(createApiSession(fmt.Sprintf("%s %s", c.cmd, c.arg)))

	session.Setenv("ANVIL_API_PORT", strconv.Itoa(client.ListenerPort()))
	session.Setenv("ANVIL_API_SESS", string(apiSess.Id()))

	if c.extraEnv != nil {
		names, values := mylog.Check3(c.extraEnvNamesAndValues())

		for i, n := range names {
			log(LogCatgFS, "sshFs.execAsync: setting env var %s=%s\n", n, values[i])
			session.Setenv(n, values[i])
		}

	}

	c1, c2 := mergeContentsInto(c.contents)

	go copyBlocks(stdout, c1, 4096, c.errs, nil)
	go copyBlocks(stderr, c2, 4096, c.errs, nil)

	ok = true
	return
}

func buildShellString(c execCtx, shell, dir, extra string) string {
	log(LogCatgFS, "buildShellString: template is '%s'\n", c.shellString)
	if c.shellString == "" {
		return fmt.Sprintf(`%s -c $'cd "%s" && %s %s%s'`,
			shell, dir, escapeSingleTicks(c.cmd), escapeSingleTicks(c.arg), extra)
	}

	s := c.shellString
	s = strings.ReplaceAll(s, "{Dir}", dir)
	s = strings.ReplaceAll(s, "{Cmd}", escapeSingleTicks(c.cmd))
	s = strings.ReplaceAll(s, "{Args}", escapeSingleTicks(c.arg))
	return s
}

func (f sshFs) maybeServeAPIOverSshClient(client *SshClient) (err error) {
	if client.userData != nil {
		log(LogCatgFS, "sshFs.maybeServeAPIOverSshClient: API already started\n")
		// We already started the API on the client's listener
		return nil
	}

	listener := mylog.Check2(client.Listener())

	log(LogCatgFS, "sshFs.maybeServeAPIOverSshClient: Serving API\n")
	go func() {
		mylog.Check(ServeAPIOnListener(listener))
	}()

	client.userData = true

	return nil
}

var winInvalidPathSyntaxErr = syscall.Errno(123)

func fileExists(path string) (ok bool, err error) {
	if _ = mylog.Check2(os.Stat(path)); err == nil {
		ok = true
	} else if errors.Is(err, fs.ErrNotExist) {
		ok = false
		err = nil
	} else if errors.Is(err, winInvalidPathSyntaxErr) {
		// On Windows if we try and Stat a unix path it causes the error "CreateFile sjc-ads-5487:5001:/nobackup/jefwill3/ws/space_nr2f_dev_135_2/: The filename, directory name, or volume label syntax is incorrect."
		// This is returned as a *fs.PathError, who's Err field is set to a syscall.Errno, which has the error number 123. So
		// we check for that here.
		// When this happens we know the path is not a valid local path.
		ok = false
		err = nil
	}
	return
}

func isDir(path string) (ok bool, err error) {
	if s := mylog.Check2(os.Stat(path)); err == nil && s.IsDir() {
		ok = true
	}
	return
}

func filenamesInDir(path string) (names []string, err error) {
	entries := mylog.Check2(os.ReadDir(path))

	names = make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if n == "." || n == ".." {
			continue
		}

		fi := mylog.Check2(os.Stat(filepath.Join(path, e.Name())))
		if err == nil && fi.IsDir() {
			n = fmt.Sprintf("%s%c", n, filepath.Separator)
		}

		names = append(names, n)
	}

	return
}

func pathIsRemote(path string) (bool, error) {
	p := mylog.Check2(NewGlobalPath(path, GlobalPathUnknown))

	return p.IsRemote(), nil
}

func escapeSingleTicks(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
