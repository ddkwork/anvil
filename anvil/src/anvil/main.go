package main

import (
	_ "embed"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"github.com/ddkwork/golibrary/mylog"

	//"net/http"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"github.com/ogier/pflag"

	"github.com/jeffwilliams/anvil/internal/ansi"
	adebug "github.com/jeffwilliams/anvil/internal/debug"
	"github.com/jeffwilliams/anvil/internal/expr"
	"github.com/jeffwilliams/anvil/internal/typeset"
)

const editorName = "anvil"

var (
	optProfile      = pflag.BoolP("profile", "p", false, "Profile the code CPU usage. The profile file location is printed to stdout.")
	optLoadDumpfile = pflag.StringP("load", "l", "", "Load state from the specified file that was created using Dump")
	optChdir        = pflag.StringP("cd", "d", "", "Change directory to the specified path before starting")
	optDebugStdout  = pflag.BoolP("dbg", "b", true, "Print debug logs to stdout")
)

func main() {
	mylog.Call(func() { run() })
}

func run() {
	parseAndValidateOptions()

	if *optChdir != "" {
		mylog.Check(os.Chdir(*optChdir))
	}

	if *optProfile {
		startProfiling(ProfileCPU)
	}
	LoadSettings()
	LoadStyle()
	// HirePlumber()//todo
	ansi.InitColors(WindowStyle.Ansi.AsColors())
	editor = NewEditor(WindowStyle)
	application = NewApplication()

	LoadSshKeys()
	initDebugging()

	go ServeLocalAPI()

	go func() {
		w := app.NewWindow()
		application.SetWindow(w)
		parms := uiLoopInitParams{
			dumpfileToLoad: *optLoadDumpfile,
			initialFiles:   pflag.Args(),
		}
		mylog.Check(loop(w, &parms))
		Exit(0)
	}()
	app.Main()

	if *optProfile {
		stopProfiling()
	}
}

func parseAndValidateOptions() {
	pflag.Parse()

	if pflag.NArg() > 0 && *optLoadDumpfile != "" {
		fmt.Printf("Filenames cannot be specified as arguments when the option --load is used\n")
		Exit(1)
	}
}

var styleLoadedFromFile bool

func LoadStyle() {
	style := mylog.Check2(LoadStyleFromConfigFile(&WindowStyle))

	log(LogCatgApp, "Loaded style from config file %s\n", StyleConfigFile())
	WindowStyle = style
	styleLoadedFromFile = true
}

var (
	settingsLoadedFromFile bool
	settings               = Settings{
		Ssh: SshSettings{
			Shell:      "sh",
			CacheSize:  5,
			CloseStdin: false,
		},
		Layout: LayoutSettings{
			EditorTag:         "Newcol Kill Putall Dump Load Exit Help ◊",
			ColumnTag:         "New Cut Paste Snarf Zerox Delcol",
			WindowTagUserArea: " Do Look ",
		},
	}
)

func LoadSettings() {
	mylog.Check(LoadSettingsFromConfigFile(&settings))

	log(LogCatgApp, "Loaded settings from config file %s\n", SettingsConfigFile())

	settingsLoadedFromFile = true
}

var plumbingLoadedFromFile bool

func HirePlumber() {
	HirePlumberUsingFile(PlumbingConfigFile())
}

func HirePlumberUsingFile(path string) {
	rules := LoadPlumbingRulesFromFile(path)
	log(LogCatgApp, "Loaded plumbing rules from config file %s\n", PlumbingConfigFile())
	plumber = NewPlumber(rules)
	plumbingLoadedFromFile = true
}

// https://colorhunt.co/palette/1624471f40681b1b2fe43f5a
var WindowStyle = Style{
	Fonts: []FontStyle{
		{
			FontName: "defaultVariableFont",
			FontFace: VariableFont,
			FontSize: 14,
		},
		{
			FontName: "defaultMonoFont",
			FontFace: MonoFont,
			FontSize: 14,
		},
	},
	TagFgColor:                MustParseHexColor("#f0f0f0"),
	TagBgColor:                MustParseHexColor("#263859"),
	TagPathBasenameColor:      MustParseHexColor("#f4a660"),
	BodyFgColor:               MustParseHexColor("#f0f0f0"),
	BodyBgColor:               MustParseHexColor("#17223B"),
	LayoutBoxFgColor:          MustParseHexColor("#9b2226"),
	LayoutBoxUnsavedBgColor:   MustParseHexColor("#9b2226"),
	LayoutBoxBgColor:          MustParseHexColor("#6B778D"),
	ScrollFgColor:             MustParseHexColor("#17223B"),
	ScrollBgColor:             MustParseHexColor("#6B778D"),
	WinBorderColor:            MustParseHexColor("#000000"),
	WinBorderWidth:            2,
	GutterWidth:               14,
	PrimarySelectionFgColor:   MustParseHexColor("#17223B"),
	PrimarySelectionBgColor:   MustParseHexColor("#b1b695"),
	ExecutionSelectionFgColor: MustParseHexColor("#17223B"),
	ExecutionSelectionBgColor: MustParseHexColor("#fa8072"),
	SecondarySelectionFgColor: MustParseHexColor("#17223B"),
	SecondarySelectionBgColor: MustParseHexColor("#fcd0a1"),
	ErrorsTagFgColor:          MustParseHexColor("#f0f0f0"),
	ErrorsTagBgColor:          MustParseHexColor("#54494C"),
	ErrorsTagFlashFgColor:     MustParseHexColor("#f0f0f0"),
	ErrorsTagFlashBgColor:     MustParseHexColor("#9b2226"),
	TabStopInterval:           30, // in pixels
	LineSpacing:               0,
	TextLeftPadding:           3,
	Syntax: SyntaxStyle{
		// Colors borrowed from vim jellybeans color scheme https://github.com/nanotech/jellybeans.vim/blob/master/colors/jellybeans.vim
		KeywordColor:      MustParseHexColor("#8fbfdc"), // jellybeans color for PreProc
		NameColor:         MustParseHexColor("#f0f0f0"), // Color names as normal text.
		StringColor:       MustParseHexColor("#99ad6a"),
		NumberColor:       MustParseHexColor("#cf6a4c"), // jellybeans Constant
		OperatorColor:     MustParseHexColor("#f0f0f0"), // Color operators as normal text
		CommentColor:      MustParseHexColor("#888888"),
		PreprocessorColor: MustParseHexColor("#c6b6ee"), // jellybeans identifier
		HeadingColor:      MustParseHexColor("#99ad6a"),
		SubheadingColor:   MustParseHexColor("#c6b6ee"),
		// InsertedColor:     MustParseHexColor("#aa3939"),
		// DeletedColor:      MustParseHexColor("#2d882d"),
		InsertedColor: MustParseHexColor("#51a151"),
		DeletedColor:  MustParseHexColor("#ca6565"),
	},
	Ansi: AnsiStyle{
		Colors: [16]Color{
			MustParseHexColor("#000000"),
			MustParseHexColor("#800000"),
			MustParseHexColor("#008000"),
			MustParseHexColor("#808000"),
			MustParseHexColor("#000080"),
			MustParseHexColor("#800080"),
			MustParseHexColor("#008080"),
			MustParseHexColor("#c0c0c0"),
			MustParseHexColor("#808080"),
			MustParseHexColor("#ff0000"),
			MustParseHexColor("#00ff00"),
			MustParseHexColor("#ffff00"),
			MustParseHexColor("#0000ff"),
			MustParseHexColor("#ff00ff"),
			MustParseHexColor("#00ffff"),
			MustParseHexColor("#ffffff"),
		},
	},
}

var (
	editor      *Editor
	application *Application
	appWindow   *app.Window
	window      *Window
	plumber     *Plumber
	debugLog    *adebug.DebugLog = adebug.New(100)
)

func dumpPanic(i interface{}) {
	fname := fmt.Sprintf("%s.panic", editorName)
	f := mylog.Check2(os.Create(fname))
	defer func() { mylog.Check(f.Close()) }()
	mylog.Check2(fmt.Fprintf(f, "panic: %v\n", i))
	mylog.Check2(fmt.Fprintf(f, "%s", string(debug.Stack())))
}

func dumpLogs() {
	fname := fmt.Sprintf("%s.panic-logs", editorName)
	f := mylog.Check2(os.Create(fname))

	defer func() { mylog.Check(f.Close()) }()
	mylog.Check2(fmt.Fprintf(f, debugLog.String()))
}

func dumpGoroutines() {
	fname := fmt.Sprintf("%s.panic-gortns", editorName)

	f := mylog.Check2(os.Create(fname))

	defer func() { mylog.Check(f.Close()) }()

	buf := make([]byte, 100000)
	sz := runtime.Stack(buf, true)
	buf = buf[0:sz]

	mylog.Check2(fmt.Fprintf(f, string(buf)))
}

func initializeEditorToCurrentDirectory() {
	col := editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	col = editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	window := col.NewWindow()
	window.LoadFile(".")
}

func initializeEditorToFiles(files []string) {
	col := editor.NewCol()
	col.Tag.SetTextStringNoUndo(settings.Layout.ColumnTag)
	for _, f := range files {
		editor.LoadFile(f)
	}
}

func initializeEditorWithDumpfile(f string) {
	var state ApplicationState
	mylog.Check(ReadState(f, &state))

	application.SetState(&state)
}

type uiLoopInitParams struct {
	dumpfileToLoad string
	initialFiles   []string
}

func loop(w *app.Window, parms *uiLoopInitParams) error {
	defer func() {
		if r := recover(); r != nil {
			dumpPanic(r)
			dumpLogs()
			dumpGoroutines()
			panic(r)
		}
	}()

	if parms.dumpfileToLoad != "" {
		initializeEditorWithDumpfile(parms.dumpfileToLoad)
	} else if len(parms.initialFiles) > 0 {
		initializeEditorToFiles(parms.initialFiles)
	} else {
		initializeEditorToCurrentDirectory()
	}

	appWindow = w

	application.SetTitle(editorName)

	invalidate := make(chan struct{}, 1)

	for {
		select {
		case e := <-w.Events():
			mylog.Check(handleEvent(e))

		case w := <-editor.WorkChan():
			done := w.Service()
			if done && w.Job() != nil {
				editor.RemoveJob(w.Job())
				if sn, ok := w.Job().(StartNexter); ok {
					sn.StartNext()
				}
			}

			select {
			case invalidate <- struct{}{}:
			default:
			}
		case <-invalidate:
			appWindow.Invalidate()
		}
	}
}

var focusSet bool

func handleEvent(e event.Event) error {
	var ops op.Ops
	switch e := e.(type) {
	case clipboard.Event:
		log(LogCatgUI, "clipboard.Event\n")
	case system.DestroyEvent:
		Exit(0)
	case system.FrameEvent:
		// fmt.Printf("Frame event at %v. Insets: %#v\n", e.Now, e.Insets)
		application.SetMetric(e.Metric)
		gtx := layout.NewContext(&ops, e)
		layoutWidgets(gtx, e.Queue)

		if !focusSet && window != nil {
			window.SetFocus(gtx)
			focusSet = true
			window = nil
		}

		// invalidateIfNeeded(&ops)

		e.Frame(gtx.Ops)
	case app.ConfigEvent:
		log(LogCatgUI, "window config changed: %v\n", e.Config)
		application.WindowConfigChanged(&e.Config)
	}
	return nil
}

//go:embed font/InputMonoCondensed-ExtraLight.ttf
var InputMonoFont []byte

//go:embed font/InputSansCondensed-ExtraLight.ttf
var InputVariableFont []byte

// Set the default font to the Input font
var MonoFont = text.FontFace{
	Font: font.Font{
		Typeface: "defaultMonoFont",
	},
	Face: MustParseTTFBytes(InputMonoFont),
	// Uncomment the below to make the default font the Go fonts.
	// Face: MustParseTTFBytes(gomono.TTF),
}

var VariableFont = text.FontFace{
	Font: font.Font{
		Typeface: "defaultVariableFont",
	},
	Face: MustParseTTFBytes(InputVariableFont),
	// Uncomment the below to make the default font the Go fonts.
	// Face: MustParseTTFBytes(goregular.TTF),
}

func MustParseTTFBytes(b []byte) font.Face {
	face := mylog.Check2(typeset.ParseTTFBytes(b))

	return face
}

type Collection []text.FontFace

func (c Collection) ContainsFont(font font.Font) bool {
	for _, f := range c {
		if f.Font == font {
			return true
		}
	}
	return false
}

func layoutWidgets(gtx layout.Context, queue event.Queue) {
	editor.Layout(gtx, queue)
}

func Exit(code int) {
	if *optProfile {
		stopProfiling()
	}
	os.Exit(code)
}

func init() {
	// editor.LoadFile("C:\\Users\\Admin\\Downloads\\anvil-src-v0.1\\anvil\\src\\anvil\\main.go")
	os.Args = []string{
		"",
		"D:\\workspace\\workspace\\app\\widget\\chapar\\ui\\widgets\\cmd\\xxx.js",
		//"C:\\Users\\Admin\\Downloads\\anvil-src-v0.1\\anvil\\src\\anvil\\main.go",
	}
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [file]\n", os.Args[0])
		fmt.Printf("Launch the Anvil text editor. If [file] is given, that file is opened.\n\n")
		fmt.Printf("Options:\n")

		pflag.PrintDefaults()
	}
}

type EditorInitializationParams struct {
	LoadDumpFile string
	InitialFile  string
}

func log(category, message string, args ...interface{}) {
	if *optDebugStdout {
		fmt.Printf(message, args...)
	}
	debugLog.Addf(category, message, args...)
}

func initDebugging() {
	expr.Debug = func(message string, args ...interface{}) {
		log(LogCatgExpr, message, args...)
	}
}

//func setCursor(name string) {
/*
		List of cursor names on X11:
		https://www.oreilly.com/library/view/x-window-system/9780937175149/ChapterD.html
	X_cursor 	0
	arrow 	2
	based_arrow_down 	4
	based_arrow_up 	6
	boat 	8
	bogosity 	10
	bottom_left_corner 	12
	bottom_right_corner 	14
	bottom_side 	16
	bottom_tee 	18
	box_spiral 	20
	center_ptr 	22
	circle 	24
	clock 	26
	coffee_mug 	28
	cross 	30
	cross_reverse 	32
	crosshair 	34
	diamond_cross 	36
	dot 	38
	dotbox 	40
	double_arrow 	42
	draft_large 	44
	draft_small 	46
	draped_box 	48
	exchange 	50
	fleur 	52
	gobbler 	54
	gumby 	56
	hand1 	58
	hand2 	60
	heart 	62
	icon 	64
	iron_cross 	66
	left_ptr 	68
	left_side 	70
	left_tee 	72
	leftbutton 	74
	ll_angle 	76
	lr_angle 	78
	man 	80
	middlebutton 	82
	mouse 	84
	pencil 	86
	pirate 	88
	plus 	90
	question_arrow 	92
	right_ptr 	94
	right_side 	96
	right_tee 	98
	rightbutton 	100
	rtl_logo 	102
	sailboat 	104
	sb_down_arrow 	106
	sb_h_double_arrow 	108
	sb_left_arrow 	110
	sb_right_arrow 	112
	sb_up_arrow 	114
	sb_v_double_arrow 	116
	shuttle 	118
	sizing 	120
	spider 	122
	spraycan 	124
	star 	126
	target 	128
	tcross 	130
	top_left_arrow 	132
	top_left_corner 	134
	top_right_corner 	136
	top_side 	138
	top_tee 	140
	trek 	142
	ul_angle 	144
	umbrella 	146
	ur_angle 	148
	watch 	150
	xterm
*/
/*
	if appWindow != nil {
		//w.SetCursorName("icon")
		appWindow.SetCursorName(pointer.CursorName(name))
	}

}
*/
