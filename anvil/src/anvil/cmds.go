package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
	"golang.org/x/image/colornames"

	"gioui.org/layout"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/ddkwork/golibrary/mylog"
)

var cmdHistory = NewCommandHistory(100)

type CommandExecutor struct {
	// source is a Window, Col or Editor.
	source interface{}
	// commands map[string]command
	commandSet
	debugCommandSet commandSet
}

type command struct {
	name      string
	do        func(ctx *CmdContext)
	shortHelp string
	longHelp  string
}

type commandSet struct {
	commands map[string]command
}

func (c *commandSet) AddCommand(name string, do func(ctx *CmdContext), shortHelp, longHelp string) {
	if c.commands == nil {
		c.commands = map[string]command{}
	}

	c.commands[name] = command{
		name:      name,
		do:        do,
		shortHelp: shortHelp,
		longHelp:  longHelp,
	}
}

func (c *commandSet) Command(name string) (cmd command, ok bool) {
	cmd, ok = c.commands[name]
	return
}

func (c *commandSet) Commands() []command {
	var l []command
	for _, v := range c.commands {
		l = append(l, v)
	}
	return l
}

func NewCommandExecutor(source interface{}) *CommandExecutor {
	ex := &CommandExecutor{
		source: source,
	}
	ex.initCommands()
	return ex
}

func (c *CommandExecutor) initCommands() {
	c.initDebugCommands()
	c.initToplevelCommands()
}

func (c *CommandExecutor) initToplevelCommands() {
	addCommand := func(name string, do func(ctx *CmdContext), shortHelp, longHelp string) {
		c.AddCommand(name, do, shortHelp, longHelp)
		AddHelp(name, longHelp)
	}

	addCommand("Del", c.CmdDel, "Delete Window", "Del closes the current window.")
	addCommand("Del!", c.CmdDelForce, "Delete Window without prompt", "Del! closes the current window. If there are unsaved changes, the user is not prompted to save them.")
	addCommand("Exit", c.CmdExit, "Exit the editor", "Exit exits the editor.")
	addCommand("New", c.CmdNew, "Make a new window or open a path", "New makes a new window or with an argument opens a path. If a window for that file is already opened, a new window for that file is not created. Otherwise, the window is opened in the column with the most free space. If new is executed with an argument the file or directory with the name of the argument is loaded into the window.")
	addCommand("Acq", c.CmdAcq, "Acquire a path", "Acq 'acquires' it's argument. It performs the same function as ALT+Right Click performs on a text object.")
	addCommand("Newcol", c.CmdNewcol, "Create a column", "Newcol creates a new column.")
	addCommand("Delcol", c.CmdDelcol, "Delete the column", "Delcol deletes the column in which it is executed.")
	addCommand("Cut", c.CmdCut, "Cut selected text", "Cut deletes the last selected text and it to the clipboard.")
	addCommand("Snarf", c.CmdSnarf, "Copy selected text", "Snarf copies the last selected text to the clipboard.")
	addCommand("Id", c.CmdId, "Show window ID", "Id prints the window ID to the +Errors window. Useful when using the API.")
	addCommand("Paste", c.CmdPaste, "Paste text", "Paste writes the text from the clipboard to the window.")
	addCommand("Put", c.CmdPut, "Save the window body", "Put writes the contents of the window body to the path that is the leftmost text in the window tag.")
	addCommand("Get", c.CmdGet, "Load the window body", "Get reads the contents of the path that is the leftmost text in the window tag and replaces the window body contents with it.")
	addCommand("Kill", c.CmdKill, "Kill a running job", "Kill kills all the jobs that are currently running that have names matching the arguments to the Kill command. If no argument is provided the first job is killed")
	addCommand("Look", c.CmdLook, "Look for a string in the window body", "Look searches for the next string in the window body that exactly matches the argument to Look.")
	addCommand("Keypass", c.CmdKeyPassword, "Specify the password used to decrypt an ssh private key file or log into a host", "Keypass is used to specify the password used to decrypt an ssh private key file. It takes two arguments: the first is the ssh filename and the second is the password. This is needed when an ssh private key file is encrypted and ssh-agent is not being used.")
	addCommand("Hostpass", c.CmdHostPassword, "Specify the password used to log into an ssh server", "Hostpass is used to specify the password used to log into an ssh server. It takes between two and four arguments. The first argument is the password. The second argument is the hostname or IP address of the server. The third argument is the username for the server; if not specified the current user's name is used. The fourth argument is the TCP port number for the server; if not specified 22 is used.")
	addCommand("Zerox", c.CmdZerox, "Clone a window", "Zerox opens a second window which is a copy of the current window")
	addCommand("Title", c.CmdTitle, "Set the editor title", "Title sets the title of the editor to it's combined arguments. The title is usually displayed by the OS window manager in the title bar.")
	addCommand("Syn", c.CmdSyntax, "Enable or disable syntax highlighting, or list supported formats", "Syntax is used to control syntax highlighting for the current window. With the argument 'off' it disables syntax highlighting, and with the argument 'list' it lists the valid supported languages. With any other argument it enables syntax highlighting and highlights the body using the language named by the argument. With no argument it attempts to analyze the text to autodetect the language.")
	addCommand("Ansi", c.CmdAnsi, "Enable or disable Ansi colors", "Ansi is used to control whether Ansi terminal color escape sequences cause coloring or not. With no argument or the 'on' it enables coloring. With the argument 'off' it disables coloring.")
	addCommand("Dump", c.CmdDump, "Save the editor's state to disk", fmt.Sprintf("Dump saves the editor's state to disk: the size of the open windows and the current value of their tags. With an argument the state is written to the file named by the argument. With no argument state is written to the file %s.dump. The state can be loaded using Load", editorName))
	addCommand("Load", c.CmdLoad, "Load the editor's state from disk", fmt.Sprintf("Load loads the editor's state from disk as written by the Dump command. With an argument the state is read from the file named by the argument. With no argument state is read from the file %s.dump", editorName))
	addCommand("Putall", c.CmdPutall, "Save all windows", "Putall executes a Put on all open windows, saving all windows.")
	addCommand("Recent", c.CmdRecent, "Display recent files", "Recent writes the list of most recently closed files to the Errors window.")
	addCommand("Mark", c.CmdMark, "Add a bookmark", "Mark saves the current cursor position in the window body with the name specified by the argument. If no argument is given it is saved with the name 'def'.")
	addCommand("Goto", c.CmdGoto, "Jump to a bookmark", "Goto sets the current cursor position in the window body to the named bookmark, created by Mark. If no argument is given it jumps to the bookmark 'def'.")
	addCommand("Marks", c.CmdMarks, "Display bookmarks", "Marks displays the currently set bookmarks to the Errors window.")
	addCommand("Marks-", c.CmdClearMarks, "Clear bookmarks", "Marks- clears all the currently set bookmarks.")
	addCommand("SaveStyle", c.CmdSaveStyle, "Save current editor style", fmt.Sprintf("SaveStyle saves the editor style information to a file: the current font and size, colors, etc. With one argument the style is saved to the file named by the argument. With no argument it is saved to %s. When the editor is started the style file %s is loaded", StyleConfigFile(), StyleConfigFile()))
	addCommand("LoadStyle", c.CmdLoadStyle, "Load editor style from file", fmt.Sprintf("LoadStyle loads the editor style information from a file: the current font and size, colors, etc. With one argument the style is loaded from the file named by the argument. With no argument it is loaded from %s. When the editor is started the style file %s is loaded", StyleConfigFile(), StyleConfigFile()))
	addCommand("LoadPlumbing", c.CmdLoadPlumbing, "Load plumbing rules from file", fmt.Sprintf("LoadPlumbing loads the plumbing rules from a file. With one argument the plumbing is loaded from the file named by the argument. With no argument it is loaded from %s. When the editor is started the plumbing file %s is loaded", PlumbingConfigFile(), PlumbingConfigFile()))
	addCommand("Help", c.CmdHelp, "Show help", "Help shows a bit of help for the editor. With no argument it lists the main commands and a brief description. With an argument displays information about that topic. The argument may be a command, which displays more detail about the command, or it may be another selected topic.")
	addCommand("◊", c.CmdInsertLozenge, "Insert a ◊ rune, or surround selection with it", "If there are no selections, insert a ◊ rune at the cursor. If there are selections, insert a ◊ before and after each selection.")
	addCommand("Rot", c.CmdRot, "Rotate selections", "Rot rotates the selections when there are multiple selections. The primary selection moves to the next selection, that one to the next and so on, with the last moving to the primary.")
	addCommand("Do", c.CmdDo, "Execute command", "Do executes it's arguments as a command; i.e. as if the arguments were selceted and executed alone. This is useful to execute commands from one window in the context of another window.")
	addCommand("About", c.CmdAbout, "About the editor", "Print information about the editor, including where some files are expected to be located")
	addCommand("Font", c.CmdFont, "Change to next font", "Change to the next font defined in the styles")
	addCommand("On", c.CmdOn, "Run command on remote host", "Run takes two or more arguments. The first is a host and directory (in the format host:directory) and the remaining arguments are the command and arguments to run.")
	addCommand("Cmds", c.CmdCmds, "List the recent external commands", "List the most recent external commands executed")
	addCommand("Cmds*", c.CmdCmdsVerbose, "List the recent external commands verbosely", "List the most recent external commands executed along with the directory they were executed in")
	addCommand("Wins", c.CmdWins, "List the open windows", "List the filenames of the open windows")
	addCommand("Undo", c.CmdUndo, "Undo the last change", "Undo the last change")
	addCommand("Redo", c.CmdRedo, "Redo the last change", "Redo the last change")
	addCommand("PrintCfg", c.CmdPrintCfg, "Print a sample config file", "Print a sample config file to +Errors. The argument specifies the file to generate:\n  ◊PrintCfg settings.toml◊ generates a settings file\n")
	addCommand("Only", c.CmdOnly, "Del other windows in this column", "When executed in a window or its tag, close the other windows in this column leaving only this window.")
	addCommand("Clr", c.CmdClr, "Clear (delete) the contents of the window body", "Clear (delete) the contents of the window body")
	addCommand("Shstr", c.CmdShstr, "Set the 'Shell String' for the current window",
		`When executed with one or more arguments, set the 'Shell String' for the current window: the template string that is used to build the command run on a remote system. It may contain these substitutions within braces:

  Dir: The window directory
  Cmd: The name of the command to be executed
  Args: Arguments to the command

The default Shell String (assuming the current shell is sh) is: sh -c $'cd "{Dir}" && {Cmd} {Args}'

When executed with no arguments, set the Shell String for the current window to the default.
`)

	addCommand("Dbg", c.CmdDbg, "Internal debugging commands", c.dbgCommandLongHelp())
	addCommand("Hidecol", c.CmdHideCol, "Hide the column", "Hidecol hides the current column.")
	addCommand("Showcol", c.CmdShowCol, "Show a column", "Showcol makes the column with the name that matches the first argument visible. If no argument is passed, the first hidden column is made visible")
	addCommand("Cols", c.CmdCols, "List columns", "Cols lists all the columns")
	addCommand("Cols*", c.CmdColsVerbose, "List columns verbosely", "Cols* lists all the columns verbosely (including the files in each column)")
	addCommand("Tint", c.CmdTint, "Colorize selections", "Tint is used to color selections of text. When executed with one argument it changes the text in all current selections to that color. The argument must be a hex color code in the form #rrggbb or a color name. When executed with no argument and selections present, it removes the coloring for text that overlap the selections. When run with no arguments and no selections it clears all tinting.")
}

func (c *CommandExecutor) dbgCommandLongHelp() string {
	var buf bytes.Buffer

	buf.WriteString("Dbg is used to run commands to help debug the internals of Anvil. The available commands are:\n")

	for _, c := range c.debugCommandSet.Commands() {
		fmt.Fprintf(&buf, "%s  (◊Help %s◊)\n\t%s\n", c.name, "Dbg "+c.name, c.shortHelp)
	}
	return buf.String()
}

func (c *CommandExecutor) initDebugCommands() {
	addCommand := func(name string, do func(ctx *CmdContext), shortHelp, longHelp string) {
		c.debugCommandSet.AddCommand(name, do, shortHelp, longHelp)
		AddHelp("Dbg "+name, longHelp)
	}

	addCommand("ProfCpu", c.CmdProfCpu, "Profile CPU usage", "Dbg ProfCpu starts writing profiling information to disk until it is executed a second time at which point it stops profiling.")
	addCommand("ProfHeap", c.CmdProfHeap, "Profile memory usage", "Dbg ProfHeap is a debug command. It starts writing profiling information to disk until it is executed a second time at which point it stops profiling.")
	addCommand("Goroutines", c.CmdGoroutines, "Print all goroutines", "Dbg Goroutines is a debug command. It writes all goroutine stacks to the errors window.")
	addCommand("Logs", c.CmdDbgLogs, "Print internal debug logs", fmt.Sprintf("Dbg Logs displays internal debug logs to the +Errors window. With no arguments it writes logs from all categories. With one or more arguments only those categories are printed. The available categories are:\n  %s",
		strings.Join(debugLogCategories, "\n  ")))
	addCommand("Pid", c.CmdDbgGetPid, "Print Anvil's PID", "Print the process ID of Anvil")
	addCommand("Psrv", c.CmdDbgPsrv, "Start the Go pprof debug server",
		`This command starts the Go pprof debug http server [1] on localhost port 6060. This server can be used to debug Anvil performance. Once started, some useful URLs to browse are:

  go tool pprof http://localhost:6060/debug/pprof/heap
  go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
  go tool pprof http://localhost:6060/debug/pprof/block
  go tool pprof http://localhost:6060/debug/pprof/mutex

[1] https://pkg.go.dev/net/http/pprof
	`)
}

func (c CommandExecutor) Do(cmd string, ctx *CmdContext) {
	cmd = strings.TrimLeft(cmd, " \t\n\r")
	rawCmd := cmd
	cmd, ctx.Args = c.split(cmd, ctx.Args)

	if len(cmd) == 0 {
		return
	}

	if cmd[0] == '|' {
		c.CmdExecPipe(cmd[1:], ctx)
		return
	} else if cmd[0] == '>' {
		c.CmdExecGt(cmd[1:], ctx)
		return
	} else if cmd[0] == '<' {
		c.CmdExecLt(cmd[1:], ctx)
		return
	} else if cmd[0] == '!' {
		c.CmdExpr(rawCmd[1:], ctx)
		return
	}

	doer, ok := c.Command(cmd)
	if ok {
		doer.do(ctx)
		return
	}

	handled := c.tryApiUserDefinedCommand(ctx, cmd)
	if handled {
		return
	}

	c.tryOsCmd(ctx, cmd)
}

func (c CommandExecutor) split(cmd string, args []string) (newcmd string, newargs []string) {
	a := strings.Fields(cmd)
	if len(a) <= 1 {
		newcmd = cmd
		newargs = args
		return
	}

	newcmd = a[0]
	newargs = a[1:]
	newargs = append(newargs, args...)
	return
}

type CmdContext struct {
	Gtx         layout.Context
	Dir         string
	Editable    *editable
	Args        []string
	Path        string
	Selections  []*selection
	rawCmd      string
	ShellString string
}

func (c CmdContext) CombinedArgs() string {
	return strings.Join(c.Args, " ")
}

func (c CommandExecutor) CmdDel(ctx *CmdContext) {
	switch v := c.source.(type) {
	case Window:
	case *Window:
		c.delWindowsOrDisplayError(v)
	}
}

func (c CommandExecutor) delWindowsOrDisplayError(wins ...*Window) (someNotDeleted bool) {
	winsNotDeleted := make([]*Window, 0, len(wins))
	for _, w := range wins {
		notDeleted := c.delWindow(w)
		if notDeleted {
			winsNotDeleted = append(winsNotDeleted, w)
		}
	}

	if len(winsNotDeleted) > 0 {
		for _, w := range winsNotDeleted {
			c.displayWindowDeletionError(w)
		}
		return true
	}
	return false
}

func (c CommandExecutor) delWindow(w *Window) (didNotDelete bool) {
	if w.col == nil {
		return
	}

	if !w.CanDelete() {
		w.SetAllowDirtyDelete(true)
		didNotDelete = true
		return
	}
	application.winIdGenerator.Free(w.Id)
	w.col.markForRemoval(w)
	return
}

func (c CommandExecutor) CmdDelForce(ctx *CmdContext) {
	switch w := c.source.(type) {
	case Window:
	case *Window:
		application.winIdGenerator.Free(w.Id)
		w.col.markForRemoval(w)
	}
}

func (c CommandExecutor) displayWindowDeletionError(w *Window) {
	editor.AppendError("", fmt.Sprintf("Refusing to delete window for %s because it has unsaved changes. Delete again if you are sure.", w.file))
}

func (c CommandExecutor) CmdExit(ctx *CmdContext) {
	wins := editor.Windows()

	someNotDeleted := c.delWindowsOrDisplayError(wins...)
	if someNotDeleted {
		return
	}
	Exit(0)
}

func (c CommandExecutor) CmdNew(ctx *CmdContext) {
	path := ""
	if len(ctx.Args) > 0 {
		path = ctx.Args[0]
	}

	if strings.TrimSpace(path) != "" {
		gpath, e := NewGlobalPath(path, GlobalPathIsFile)
		d, err2 := NewGlobalPath(ctx.Dir, GlobalPathIsDir)
		if e == nil && err2 == nil {
			gpath = gpath.MakeAbsoluteRelativeTo(d)
			if e == nil {
				path = gpath.String()
			}
		}
	}

	var w *Window
	if path != "" {
		w = editor.FindWindowForFile(path)
		if w != nil {
			// TODO: Warp pointer to here
			w.SetFocus(ctx.Gtx)
			w.GrowIfBodyTooSmall()
			return
		}
	}

	w = editor.NewWindow(c.column())
	w.markTextAsUnchanged()
	w.SetFilenameAndTag(path, typeFile)

	finder := NewFileFinder(w)

	//dirOfError := func() string {
	//	d, err2 := finder.WindowDir()
	//	if err2 != nil {
	//		d = ""
	//	}
	//	return d
	//}

	if path == "" {
		w.SetFocus(ctx.Gtx)
		return
	}

	realpath, _ := mylog.Check3(finder.Find(path))

	w.LoadFile(realpath.String())
	//if err != nil {
	//	e, ok := err.(*fs.PathError)
	//	// Don't consider the file not existing as fatal, in case of the New command
	//	if ok && !errors.Is(e, fs.ErrNotExist) {
	//		w.col.markForRemoval(w)
	//		editor.AppendError(dirOfError(), err.Error())
	//		return
	//	}
	//}

	w.SetFocus(ctx.Gtx)
}

func (c CommandExecutor) CmdAcq(ctx *CmdContext) {
	path := ""
	if len(ctx.Args) > 0 {
		path = ctx.CombinedArgs()
	}

	if strings.TrimSpace(path) == "" {
		w := editor.NewWindow(c.column())
		w.markTextAsUnchanged()
		w.SetFilenameAndTag(path, typeFile)
		w.SetFocus(ctx.Gtx)
		return
	}

	if plumber != nil {
		plumbed := mylog.Check2(plumber.Plumb(path, &c, ctx))

		if plumbed {
			return
		}
	}

	path, seek := mylog.Check3(parseSeekFromFilename(path))

	w, _ := c.source.(*Window)
	finder := NewFileFinder(w)

	realpath, _ := mylog.Check3(finder.Find(path))

	var opts LoadFileOpts
	if !seek.empty() {
		opts = LoadFileOpts{GoTo: seek, SelectBehaviour: selectText, GrowBodyBehaviour: dontGrowBodyIfTooSmall}
	}
	w = editor.LoadFileOpts(realpath.String(), opts)
	if w != nil {
		w.SetFocus(ctx.Gtx)
	}
}

func (c CommandExecutor) column() *Col {
	var col *Col

	switch v := c.source.(type) {
	case Window:
	case *Window:
		col = v.col
	case Col:
		col = &v
	case *Col:
		col = v
	}

	return col
}

func (c CommandExecutor) CmdDelcol(ctx *CmdContext) {
	switch v := c.source.(type) {
	case Col:
	case *Col:
		editor.markForRemoval(v)
	}
}

func (c CommandExecutor) CmdNewcol(ctx *CmdContext) {
	col := editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
}

func addCommandToHistory(dir, cmd, arg string) *CommandHistoryEntry {
	return cmdHistory.Started(dir, fmt.Sprintf("%s %s", cmd, arg))
}

func markCommandCompletedInHistory(e *CommandHistoryEntry) {
	cmdHistory.Completed(e)
}

func setExitCodeInHistory(e *CommandHistoryEntry, c int) {
	cmdHistory.SetExitCode(e, c)
}

func (c CommandExecutor) tryApiUserDefinedCommand(ctx *CmdContext, command string) (handled bool) {
	winId := -1
	switch v := c.source.(type) {
	case Window:
	case *Window:
		winId = v.Id
	}

	return apiHandleCommand(winId, command, ctx.Args)
}

func printErrs(c chan error) (d chan error) {
	d = make(chan error)
	go func() {
		for e := range d {
			log(LogCatgCmd, "CommandExecutor: command got error %v %T %T\n", e, e, errors.Unwrap(e))
			if ex, ok := e.(*exec.ExitError); ok {
				log(LogCatgCmd, "CommandExecutor: command got error; exit code: %d\n", ex.ExitCode())
			}
			c <- e
		}
		close(c)
	}()
	return
}

func snoopAndSaveFirstError(c chan error, entry *CommandHistoryEntry) (d chan error) {
	d = make(chan error)
	go func() {
		for e := range d {
			log(LogCatgCmd, "Snooped an error and it is a %T\n", e)
			switch t := e.(type) {
			case *exec.ExitError:
				setExitCodeInHistory(entry, t.ExitCode())
			case *ssh.ExitError:
				setExitCodeInHistory(entry, t.ExitStatus())
			}
			c <- e
		}
		close(c)
	}()
	return
}

func (c CommandExecutor) tryOsCmd(ctx *CmdContext, command string) {
	dir := ctx.Dir

	sfs := mylog.Check2(GetFs(dir))

	load := NewDataLoad()

	done := make(chan struct{})

	ec := execCtx{
		dir:         dir,
		cmd:         command,
		arg:         ctx.CombinedArgs(),
		contents:    load.Contents,
		errs:        load.Errs,
		kill:        load.Kill,
		done:        done,
		shellString: ctx.ShellString,
	}
	c.setExtraEnv(ctx, &ec)

	hist := addCommandToHistory(dir, ec.cmd, ec.arg)
	ec.errs = snoopAndSaveFirstError(ec.errs, hist)
	mylog.Check(sfs.execAsync(ec))

	go func() {
		<-done
		markCommandCompletedInHistory(hist)
	}()

	wl := &WindowDataLoad{
		DataLoad:          *load,
		Win:               NewWindowHolderForName(editor.ErrorsFileNameOf(dir)),
		Jobname:           command,
		Tail:              true,
		GrowBodyBehaviour: growBodyIfTooSmall,
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)
}

func (c CommandExecutor) setExtraEnv(ctx *CmdContext, ex *execCtx) {
	localPath := func(w *Window) string {
		var dirState GlobalPathDirState

		switch w.fileType {
		case typeUnknown:
			dirState = GlobalPathUnknown
		case typeFile:
			dirState = GlobalPathIsFile
		case typeDir:
			dirState = GlobalPathIsDir
		}

		glb := mylog.Check2(NewGlobalPath(w.file, dirState))

		return glb.Path()
	}

	localizeDir := func(dir string) string {
		glb := mylog.Check2(NewGlobalPath(dir, GlobalPathIsDir))

		return glb.Path()
	}

	base := func(path string) string {
		glb := mylog.Check2(NewGlobalPath(path, GlobalPathUnknown))

		return glb.Base()
	}

	winId := ""
	winGlobalPath := ""
	winLocalPath := ""
	winGlobalDir := ""
	winLocalDir := ""
	winPathBase := ""
	switch v := c.source.(type) {
	case Window:
	case *Window:
		winId = strconv.Itoa(v.Id)
		winGlobalPath = v.file
		winLocalPath = localPath(v)
		winGlobalDir = ctx.Dir
		winLocalDir = localizeDir(ctx.Dir)
		winPathBase = base(v.file)
	}

	ex.extraEnv = []string{
		fmt.Sprintf("ANVIL_WIN_ID=%s", winId),
		fmt.Sprintf("ANVIL_WIN_GLOBAL_PATH=%s", winGlobalPath),
		fmt.Sprintf("ANVIL_WIN_LOCAL_PATH=%s", winLocalPath),
		fmt.Sprintf("ANVIL_WIN_GLOBAL_DIR=%s", winGlobalDir),
		fmt.Sprintf("ANVIL_WIN_LOCAL_DIR=%s", winLocalDir),
		fmt.Sprintf("f=%s", winLocalPath),
		fmt.Sprintf("b=%s", winPathBase),
		fmt.Sprintf("d=%s", winLocalDir),
	}

	for k, v := range settings.Ssh.Env {
		ex.extraEnv = append(ex.extraEnv, fmt.Sprintf("%s=%s", k, v))
	}
}

func (c CommandExecutor) CmdCut(ctx *CmdContext) {
	// editor.cutLastSelection(ctx.Gtx)
	editor.cutAllSelectionsFromLastSelectedEditable(ctx.Gtx)
}

func (c CommandExecutor) CmdSnarf(ctx *CmdContext) {
	// editor.copyLastSelection(ctx.Gtx)
	editor.copyAllSelectionsFromLastSelectedEditable(ctx.Gtx)
}

func (c CommandExecutor) CmdId(ctx *CmdContext) {
	editor.AppendError("", fmt.Sprintf("%p", ctx.Editable.Tag()))
}

func (c CommandExecutor) CmdPaste(ctx *CmdContext) {
	editor.pasteToFocusedEditable(ctx.Gtx)
}

func (c CommandExecutor) CmdPut(ctx *CmdContext) {
	switch v := c.source.(type) {
	case Window:
	case *Window:
		v.Put()
	}
}

func (c CommandExecutor) CmdGet(ctx *CmdContext) {
	switch v := c.source.(type) {
	case Window:
	case *Window:
		v.Get()
		v.SetFocus(ctx.Gtx)
	}
}

func (c CommandExecutor) CmdKill(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.KillJob("")
		return
	}

	for _, s := range ctx.Args {
		editor.KillJob(s)
	}
}

func (c CommandExecutor) CmdLook(ctx *CmdContext) {
	needle := ctx.CombinedArgs()
	ctx.Editable.SearchAndUpdateEditable(ctx.Gtx, needle, ctx.Editable.firstCursorIndex(), Forward)
	ctx.Editable.SetFocus(ctx.Gtx)
}

func (c CommandExecutor) CmdKeyPassword(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", "Not enough arguments to Keypass")
		return
	}
	file := ctx.Args[0]
	pass := ctx.Args[1]
	sshClientCache.SetKeyfilePassword(file, pass)
}

func (c CommandExecutor) CmdHostPassword(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", "Not enough arguments to Hostpass")
		return
	}

	pass := ctx.Args[0]
	host := ctx.Args[1]
	user := ""
	port := ""
	if len(ctx.Args) > 2 {
		user = ctx.Args[2]
	}
	if len(ctx.Args) > 3 {
		port = ctx.Args[3]
	}
	sshClientCache.SetSshHopPassword(user, host, port, pass)
}

func (c CommandExecutor) CmdZerox(ctx *CmdContext) {
	if editor.focusedWindow == nil {
		return
	}

	mylog.Check2(editor.focusedWindow.Zerox())
}

func (c CommandExecutor) CmdTitle(ctx *CmdContext) {
	if len(ctx.Args) < 1 {
		application.SetTitle(editorName)
	}

	application.SetTitle(ctx.CombinedArgs())
}

func (c CommandExecutor) textToPipe(ctx *CmdContext) (text []string, selections []*selection) {
	if ctx.Editable.SelectionsPresent() {
		for _, sel := range ctx.Editable.selectionsInDisplayOrder() {
			text = append(text, ctx.Editable.textOfSelection(sel))
			selections = append(selections, sel)
		}
		return
	}

	text = []string{ctx.Editable.String()}
	return
}

func (c CommandExecutor) CmdExecPipe(command string, ctx *CmdContext) {
	log(LogCatgCmd, "CommandExecutor.CmdExecPipe: running command %s\n", command)

	text, sels := c.textToPipe(ctx)
	dir := ctx.Dir

	sfs := mylog.Check2(GetFs(dir))

	for i, t := range text {
		sel := (*selection)(nil)
		if sels != nil && i < len(sels) {
			sel = sels[i]
		}
		c.execPipeForOneSelection(command, ctx, dir, t, sel, sfs)

	}
}

func (c CommandExecutor) execPipeForOneSelection(command string, ctx *CmdContext, dir string, text string, sel *selection, sfs simpleFs) {
	load := NewDataLoad()

	ec := execCtx{
		dir:      dir,
		cmd:      command,
		arg:      ctx.CombinedArgs(),
		stdin:    []byte(text),
		contents: load.Contents,
		errs:     load.Errs,
		kill:     load.Kill,
	}
	c.setExtraEnv(ctx, &ec)
	mylog.Check(sfs.execAsync(ec))

	var makeWork func(job Job, ed *editable, data []byte, first bool) Work
	if sel != nil {
		makeWork = func(job Job, ed *editable, data []byte, first bool) Work {
			return &edAppendToSelection{job: job, ed: ed, data: data, first: first, sel: sel}
		}
	} else {
		makeWork = func(job Job, ed *editable, data []byte, first bool) Work {
			return &edAppend{job: job, ed: ed, data: data, first: first}
		}
	}

	wl := &EditableModify{
		DataLoad: *load,
		Jobname:  command,
		Editable: ctx.Editable,
		MakeWork: makeWork,
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)
}

func (c CommandExecutor) CmdExecGt(command string, ctx *CmdContext) {
	// This code is a little complex. We want to support running the >command on multiple selections,
	// but also want the output in the +Errors window generated for each selection to appear in the order
	// that the selections appear in the input text. If we just ran the commands asynchronously the output
	// could intermix or appear in the wrong order. To solve this we make a linked list of jobs, one per
	// selection and execute then in order. When the first one completes, the editor checks if there is another
	// in the list and executes that. The list nodes are GtExecutor structs, and each may or may not have
	// it's next property set.
	//
	// Since the work and jobs for loading the data into the editable are separate entities from the GtExecutor,
	// and it is the GtExecutor that knows what job to run next, we need a way for the work's job to
	// refer to the current GtExecutor so that we can get the next when the job completes. We do this
	// by overriding the Job that the WindowDataLoad usually returns to be a GtExecutorJob that is a
	// facade for the WindowDataLoad for the purposes of Killing and Naming the job, but that implements
	// a StartNexter so that we can start the next job when the current one ends.

	log(LogCatgCmd, "CommandExecutor.CmdExecGt: running command %s\n", command)

	text, _ := c.textToPipe(ctx)
	dir := ctx.Dir

	sfs := mylog.Check2(GetFs(dir))

	var first, last *GtExecutor

	for _, t := range text {

		executor := c.gtExecutorForOneSelection(command, ctx, dir, t, sfs)

		if executor == nil {
			continue
		}

		if last != nil {
			last.next = executor
			last = executor
			continue
		}

		if first == nil {
			first = executor
			last = executor
			continue
		}
	}

	/*log(LogCatgCmd,"CommandExecutor.CmdExecGt: job list:\n")
	for n := first; n != nil; n = n.next {
		log(LogCatgCmd,"  %p\n", n)
	}*/

	if first != nil {
		first.Start()
	}
}

func (c CommandExecutor) gtExecutorForOneSelection(command string, ctx *CmdContext, dir string, text string, sfs simpleFs) *GtExecutor {
	load := NewDataLoad()

	ec := execCtx{
		dir:      dir,
		cmd:      command,
		arg:      ctx.CombinedArgs(),
		stdin:    []byte(text),
		contents: load.Contents,
		errs:     load.Errs,
		kill:     load.Kill,
	}
	c.setExtraEnv(ctx, &ec)

	ge := &GtExecutor{
		load:    load,
		execCtx: ec,
		sfs:     sfs,
	}

	return ge
}

type GtExecutor struct {
	load    *DataLoad
	execCtx execCtx
	sfs     simpleFs
	next    *GtExecutor
}

func (g GtExecutor) StartNext() {
	if g.next != nil {
		g.next.Start()
	}
}

func (g *GtExecutor) Start() {
	log(LogCatgCmd, "GtExecutor.Start: called for %p\n", &g)
	mylog.Check(g.sfs.execAsync(g.execCtx))

	wl := &WindowDataLoad{
		DataLoad:          *g.load,
		Win:               NewWindowHolderForName(editor.ErrorsFileNameOf(g.execCtx.dir)),
		Jobname:           g.execCtx.cmd,
		Tail:              true,
		GrowBodyBehaviour: growBodyIfTooSmall,
	}

	wl.Start(editor.WorkChan())
	j := &GtExecutorJob{
		executor:    g,
		winDataLoad: wl,
	}
	wl.Job = j
	editor.AddJob(j)
}

type GtExecutorJob struct {
	executor    *GtExecutor
	winDataLoad *WindowDataLoad
}

func (j GtExecutorJob) Kill() {
	j.winDataLoad.Kill()
}

func (j GtExecutorJob) Name() string {
	return j.winDataLoad.Name()
}

func (j GtExecutorJob) StartNext() {
	j.executor.StartNext()
}

func (c CommandExecutor) CmdExecLt(command string, ctx *CmdContext) {
	log(LogCatgCmd, "CommandExecutor.CmdExecLt: running command %s\n", command)

	dir := ctx.Dir

	sfs := mylog.Check2(GetFs(dir))

	load := NewDataLoad()

	ec := execCtx{
		dir:      dir,
		cmd:      command,
		arg:      ctx.CombinedArgs(),
		contents: load.Contents,
		errs:     load.Errs,
		kill:     load.Kill,
	}
	c.setExtraEnv(ctx, &ec)
	mylog.Check(sfs.execAsync(ec))

	wl := &EditableModify{
		DataLoad: *load,
		Jobname:  command,
		Editable: ctx.Editable,
		MakeWork: func(job Job, ed *editable, data []byte, first bool) Work {
			return &edInsertText{job: job, ed: ed, data: data}
		},
	}

	wl.Start(editor.WorkChan())

	editor.AddJob(wl)
}

type EditableModify struct {
	DataLoad
	Jobname  string
	Editable *editable
	MakeWork func(job Job, ed *editable, data []byte, first bool) Work
}

func (f *EditableModify) Start(c chan Work) {
	go f.pump(c)
}

func (f *EditableModify) pump(c chan Work) {
	/*
		For ssh execution or loading we might not know if there is an error until
		we call wait at the end of the session at which point we might have already closes
		the contents.
	*/
	contentsClosed := false
	errsClosed := false
	workIsDone := func() bool {
		return contentsClosed && errsClosed
	}

	firstAppend := true

	log(LogCatgCmd, "EditableSelectionReplace.pump: started\n")
FOR:
	for {
		select {
		case x, ok := <-f.Contents:
			if !ok {
				log(LogCatgCmd, "EditableSelectionReplace.pump: contents closed\n")
				contentsClosed = true
				f.Contents = nil
				if workIsDone() {
					break FOR
				}
				break
			}

			work := f.MakeWork(f, f.Editable, x, firstAppend)
			c <- work
			// c <- &edAppendToSelection{job: f, ed: f.Editable, data: x, first: firstAppend}
			firstAppend = false
		case x, ok := <-f.Errs:
			if !ok {
				log(LogCatgCmd, "EditableSelectionReplace.pump: errs closed\n")
				errsClosed = true
				f.Errs = nil
				if workIsDone() {
					break FOR
				}
				break
			}
			log(LogCatgCmd, "EditableSelectionReplace.pump: Got an error: %v (%T)\n", x, x)
			if e, ok := x.(*fs.PathError); ok {
				log(LogCatgCmd, "  (%T)\n", e)
			}

			c <- &winLoadErr{job: f, err: x}
			// break FOR
		}
	}

	c <- &jobDone{job: f}
}

func (l *EditableModify) Kill() {
	select {
	case l.DataLoad.Kill <- struct{}{}:
	default:
	}
}

func (l *EditableModify) Name() string {
	return l.Jobname
}

type edAppendToSelection struct {
	job   Job
	ed    *editable
	data  []byte
	first bool
	sel   *selection
}

func (l edAppendToSelection) Service() (done bool) {
	if l.first {
		// l.ed.replacePrimarySelectionWith(string(l.data))
		l.ed.replaceSelectionWith(l.sel, string(l.data))
	} else {
		l.ed.appendToSelection(l.sel, string(l.data))
	}

	return false
}

func (l edAppendToSelection) Job() Job {
	return l.job
}

type edAppend struct {
	job   Job
	ed    *editable
	data  []byte
	first bool
}

func (l edAppend) Service() (done bool) {
	if l.first {
		l.ed.SetText(l.data)
	} else {
		l.ed.Append(l.data)
	}

	return false
}

func (l edAppend) Job() Job {
	return l.job
}

type jobDone struct {
	job Job
}

func (l jobDone) Service() (done bool) {
	return true
}

func (l jobDone) Job() Job {
	return l.job
}

type edInsertText struct {
	job  Job
	ed   *editable
	data []byte
}

func (l edInsertText) Service() (done bool) {
	l.ed.InsertText(string(l.data))
	return false
}

func (l edInsertText) Job() Job {
	return l.job
}

func (c CommandExecutor) CmdSyntax(ctx *CmdContext) {
	if len(ctx.Args) > 0 && ctx.Args[0] == "list" {
		names := lexers.Names(true)
		msg := fmt.Sprintf("syntax highlighting languages:\n%s\n", strings.Join(names, "\n"))
		editor.AppendError("", msg)
		return
	}

	switch v := c.source.(type) {
	case Window:
	case *Window:
		if len(ctx.Args) < 1 {
			v.Body.SetSyntaxAnalyse(true)
			return
		}

		if ctx.Args[0] == "off" {
			v.Body.DisableSyntax()
			v.Body.HighlightSyntax()
			return
		}

		v.Body.SetSyntaxLanguage(ctx.Args[0])
		v.Body.HighlightSyntax()
	}
}

func (c CommandExecutor) CmdAnsi(ctx *CmdContext) {
	on := true
	if len(ctx.Args) > 0 {
		switch ctx.Args[0] {
		case "off":
			on = false
		case "on":
			on = true
		default:
			return
		}
	}

	if ctx.Editable != nil {
		ctx.Editable.ColorizeAnsiEscapes(on)
	}
}

func (c CommandExecutor) determineDumpFilename(ctx *CmdContext) string {
	filename := fmt.Sprintf("%s.dump", editorName)

	if len(ctx.Args) >= 1 {
		filename = ctx.CombinedArgs()
	}

	return filename
}

func (c CommandExecutor) CmdDump(ctx *CmdContext) {
	state := application.State()
	filename := c.determineDumpFilename(ctx)
	mylog.Check(WriteState(filename, state))
}

func (c CommandExecutor) CmdLoad(ctx *CmdContext) {
	filename := c.determineDumpFilename(ctx)
	var state ApplicationState
	mylog.Check(ReadState(filename, &state))

	application.SetState(&state)
}

func (c CommandExecutor) CmdProfCpu(ctx *CmdContext) {
	c.CmdProf(ctx, ProfileCPU)
}

func (c CommandExecutor) CmdProfHeap(ctx *CmdContext) {
	c.CmdProf(ctx, ProfileHeap)
}

func (c CommandExecutor) CmdProf(ctx *CmdContext, what ProfileCategory) {
	if isProfiling() {
		stopProfiling()
	} else {
		startProfiling(what)
	}
}

func (c CommandExecutor) CmdGoroutines(ctx *CmdContext) {
	buf := make([]byte, 100000)
	sz := runtime.Stack(buf, true)
	buf = buf[0:sz]
	editor.AppendError("", string(buf))
}

func (c CommandExecutor) CmdPutall(ctx *CmdContext) {
	editor.Putall()
}

func (c CommandExecutor) CmdRecent(ctx *CmdContext) {
	s := strings.Join(editor.RecentFiles(), "\n")
	editor.AppendError("", s)
}

func (c CommandExecutor) CmdExpr(cmd string, ctx *CmdContext) {
	handler := ctx.Editable.makeExprHandler()

	win, _ := c.source.(*Window)
	executor := NewEditableExprExecutor(ctx.Editable, win, ctx.Dir, handler)
	executor.Do(cmd)
}

func (c CommandExecutor) CmdMark(ctx *CmdContext) {
	file := ""

	switch v := c.source.(type) {
	case Window:
	case *Window:
		file = v.file
	default:
		return
	}

	if ctx.Editable == nil {
		return
	}

	markName := "def"
	if len(ctx.Args) > 0 {
		markName = ctx.Args[0]
	}

	editor.Marks.Set(markName, file, ctx.Editable.firstCursorIndex())
}

func (c CommandExecutor) CmdGoto(ctx *CmdContext) {
	markName := "def"
	if len(ctx.Args) > 0 {
		markName = ctx.Args[0]
	}

	file, seek, ok := editor.Marks.Seek(markName)
	if ok {
		editor.LoadFileOpts(file, LoadFileOpts{GoTo: seek, SelectBehaviour: dontSelectText})
	}
}

func (c CommandExecutor) CmdMarks(ctx *CmdContext) {
	s := editor.Marks.String()
	s = fmt.Sprintf("Marks:\n%s", s)
	editor.AppendError("", s)
}

func (c CommandExecutor) CmdClearMarks(ctx *CmdContext) {
	editor.Marks.Clear()
}

func (c CommandExecutor) CmdSaveStyle(ctx *CmdContext) {
	file := StyleConfigFile()
	if len(ctx.Args) > 0 {
		file = ctx.CombinedArgs()
	}

	log(LogCatgCmd, "Saved style to file %s\n", file)
	mylog.Check(SaveCurrentStyleToFile(file))
}

func (c CommandExecutor) CmdLoadStyle(ctx *CmdContext) {
	file := StyleConfigFile()
	if len(ctx.Args) > 0 {
		file = ctx.CombinedArgs()
	}

	log(LogCatgCmd, "Loading style from file %s\n", file)
	mylog.Check(LoadCurrentStyleFromFile(file, &WindowStyle))
}

func (c CommandExecutor) CmdLoadPlumbing(ctx *CmdContext) {
	file := PlumbingConfigFile()
	if len(ctx.Args) > 0 {
		file = ctx.CombinedArgs()
	}

	log(LogCatgCmd, "Loading plumbing rules from file %s\n", file)
	HirePlumberUsingFile(file)
}

func (c CommandExecutor) CmdInsertLozenge(ctx *CmdContext) {
	if ctx.Editable != nil && editor.focusedEditable != nil {
		e := editor.focusedEditable
		e.InsertLozenge()
	}
}

func (c CommandExecutor) CmdHelp(ctx *CmdContext) {
	if len(ctx.Args) > 0 {
		t := Help(ctx.CombinedArgs())
		if t == "" {
			t = "No help for that."
		}
		editor.AppendError("", t)
		editor.AppendError("", "\n")
		return
	}

	var text bytes.Buffer
	fmt.Fprintf(&text, "%s", topLevelHelpString())

	var names []string
	for k := range c.commands {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, k := range names {
		v := c.commands[k]
		fmt.Fprintf(&text, "%s  (◊Help %s◊)\n\t%s\n", k, k, v.shortHelp)
	}
	text.WriteRune('\n')

	editor.AppendError("", text.String())
}

func (c CommandExecutor) CmdRot(ctx *CmdContext) {
	ctx.Editable.RotateSelections()
}

func (c CommandExecutor) CmdDo(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		return
	}

	cmd := ctx.Args[0]
	args := ctx.Args[1:]
	ctx.Args = args

	c.Do(cmd, ctx)
}

func (c CommandExecutor) CmdAbout(ctx *CmdContext) {
	wasLoaded := "was loaded on startup"
	wasntLoaded := "was not loaded on startup"

	loadedStr := func(loaded bool) string {
		if loaded {
			return wasLoaded
		} else {
			return wasntLoaded
		}
	}

	var text bytes.Buffer
	fmt.Fprintf(&text, "%s was written by Jeff Williams\n\n", strings.Title(editorName))
	fmt.Fprintf(&text, "Version: %s %s\n", buildVersion, buildTime)
	fmt.Fprintf(&text, "Config directory: %s\n", ConfDir)
	fmt.Fprintf(&text, "Settings file: %s (%s)\n", SettingsConfigFile(), loadedStr(settingsLoadedFromFile))
	fmt.Fprintf(&text, "Style config file: %s (%s)\n", StyleConfigFile(), loadedStr(styleLoadedFromFile))
	fmt.Fprintf(&text, "SSH key directory: %s\n", SshKeyDir())
	fmt.Fprintf(&text, "Plumbing config file: %s (%s)\n", PlumbingConfigFile(), loadedStr(plumbingLoadedFromFile))
	fmt.Fprintf(&text, "API listener port: %d\n", LocalAPIPort())

	sshKeys := sshClientCache.Keys()
	sshEntries := sshClientCache.Entries()
	if len(sshKeys) > 0 {
		fmt.Fprintf(&text, "Cached SSH connections:\n")
		for i, k := range sshKeys {
			fmt.Fprintf(&text, "  %s\n", k)
			if i < len(sshEntries) && len(sshEntries) > 0 {
				fmt.Fprintf(&text, "    API listener port: %d\n", sshEntries[i].client.ListenerPort())
			}
		}
	} else {
		fmt.Fprintf(&text, "No cached SSH connections\n")
	}

	apiSessions := getApiSessions()
	if len(apiSessions) > 0 {
		fmt.Fprintf(&text, "API sessions:\n")
		for _, e := range apiSessions {
			s := strings.Join(e.userDefinedCommands, ", ")
			if len(s) > 0 {
				s = fmt.Sprintf(" user-defined commands: [%s]", s)
			}
			fmt.Fprintf(&text, "  %s %s%s\n", e.Cmd(), e.Id(), s)
		}
	} else {
		fmt.Fprintf(&text, "No API sessions\n")
	}

	editor.AppendError("", text.String())
}

func (c CommandExecutor) CmdFont(ctx *CmdContext) {
	switch v := c.source.(type) {
	case Window:
	case *Window:
		v.Body.NextFont()
	}
}

func (c CommandExecutor) CmdOn(ctx *CmdContext) {
	if len(ctx.Args) < 2 {
		editor.AppendError("", "The On command needs at least two arguments: the directory and the command")
		return
	}

	dir := ctx.Args[0]
	cmd := ctx.Args[1]
	ctx.Args = ctx.Args[2:]
	ctx.Dir = dir

	c.tryOsCmd(ctx, cmd)
}

func (c CommandExecutor) CmdCmds(ctx *CmdContext) {
	editor.AppendError("", cmdHistory.String(NotVerbose))
}

func (c CommandExecutor) CmdCmdsVerbose(ctx *CmdContext) {
	editor.AppendError("", cmdHistory.String(Verbose))
}

func (c CommandExecutor) CmdUndo(ctx *CmdContext) {
	ctx.Editable.Undo(ctx.Gtx)
}

func (c CommandExecutor) CmdRedo(ctx *CmdContext) {
	ctx.Editable.Redo(ctx.Gtx)
}

func (c CommandExecutor) CmdPrintCfg(ctx *CmdContext) {
	if len(ctx.Args) < 1 {
		editor.AppendError("", "The PrintCfg command needs an argument.")
		return
	}

	fname := ctx.Args[0]

	switch fname {
	case "settings.toml":
		editor.AppendError("", GenerateSampleSettings())
	}
}

func (c CommandExecutor) CmdWins(ctx *CmdContext) {
	var paths []string
	for _, win := range editor.Windows() {
		path, _, _ := mylog.Check4(win.Tag.Parts())

		paths = append(paths, path)
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i] < paths[j]
	})

	for _, path := range paths {
		editor.AppendError("", path)
	}
}

func (c CommandExecutor) CmdOnly(ctx *CmdContext) {
	switch v := c.source.(type) {
	case Window:
	case *Window:
		if v.col == nil {
			return
		}

		wins := make([]*Window, 0, len(v.col.Windows))
		for _, w := range v.col.Windows {
			if w == v {
				continue
			}
			wins = append(wins, w)
		}

		c.delWindowsOrDisplayError(wins...)
	}
}

func (c CommandExecutor) CmdClr(ctx *CmdContext) {
	ctx.Editable.SetText([]byte{})
	ctx.Editable.ClearManualHighlights()
}

func (c CommandExecutor) CmdShstr(ctx *CmdContext) {
	win, ok := c.source.(*Window)
	if !ok {
		editor.AppendError("", "Shstr only works in window tags or bodies")
		return
	}

	b := mylog.Check2(isRemoteFilenameOrDir(ctx.Dir))
	if !b {
		editor.AppendError("", "Shstr only works for remote files")
		return
	}

	if len(ctx.Args) == 0 {
		win.Body.adapter.setShellString("")
		win.Tag.adapter.setShellString("")
		return
	}

	win.Body.adapter.setShellString(ctx.CombinedArgs())
	win.Tag.adapter.setShellString(ctx.CombinedArgs())
}

func (c CommandExecutor) CmdDbg(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.AppendError("", "Dbg expects at least one argument")
		return
	}

	doer, ok := c.debugCommandSet.Command(ctx.Args[0])
	if !ok {
		editor.AppendError("", fmt.Sprintf("There is no such debug command as %s", ctx.Args[0]))
		return
	}

	ctx.Args = ctx.Args[1:]
	doer.do(ctx)
}

func (c CommandExecutor) CmdDbgLogs(ctx *CmdContext) {
	msg := debugLog.String(ctx.Args...)
	editor.AppendError("", msg)
}

func (c CommandExecutor) CmdDbgGetPid(ctx *CmdContext) {
	os.Getpid()
	msg := fmt.Sprintf("pid: %d", os.Getpid())
	editor.AppendError("", msg)
}

func (c CommandExecutor) CmdDbgPsrv(ctx *CmdContext) {
	startPprofDebugServer()
}

func (c CommandExecutor) CmdHideCol(ctx *CmdContext) {
	var col *Col
	switch v := c.source.(type) {
	case *Col:
		col = v
	case *Window:
		col = v.col
	}

	if col == nil {
		return
	}

	col.SetVisible(false)
}

func (c CommandExecutor) CmdShowCol(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		editor.SetFirstHiddenColVisible()
		return
	}

	name := ctx.CombinedArgs()
	editor.SetColVisible(name)
}

func (c CommandExecutor) CmdCols(ctx *CmdContext) {
	editor.AppendError("", editor.ListCols(false, false))
}

func (c CommandExecutor) CmdColsVerbose(ctx *CmdContext) {
	editor.AppendError("", editor.ListCols(true, true))
}

func (c CommandExecutor) CmdTint(ctx *CmdContext) {
	if len(ctx.Args) == 0 {
		if ctx.Editable.SelectionsPresent() {
			ctx.Editable.ClearSelectedManualHighlights()
		} else {
			ctx.Editable.ClearManualHighlights()
		}
		return
	}

	if ctx.Args[0] == "list" {
		c.appendColorNamesInColor(ctx)
		return
	}

	color, ok := ColorFromName(ctx.Args[0])
	if ok {
		ctx.Editable.AddManualHighlightForEachSelection(color)
		return
	}

	mylog.Check2(ParseHexColor(ctx.Args[0]))

	ctx.Editable.AddManualHighlightForEachSelection(color)
}

func (c CommandExecutor) appendColorNamesInColor(ctx *CmdContext) {
	fname := editor.ErrorsFileNameOf("")
	win := editor.FindOrCreateWindow(fname)

	for _, n := range colornames.Names {
		str := "▆▆▆"
		start := win.Body.text.Len()
		end := start + utf8.RuneCountInString(str)
		color, ok := ColorFromName(n)
		line := fmt.Sprintf("%s %s\n", str, n)
		editor.AppendError("", line)
		if ok {
			win.Body.AddManualHighlight(start, end, color)
		}
	}
	return
}
