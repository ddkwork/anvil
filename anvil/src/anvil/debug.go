package main

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/ddkwork/golibrary/mylog"
)

const (
	LogCatgApp        = "Application"
	LogCatgUI         = "UI"
	LogCatgEd         = "Editable"
	LogCatgSyntax     = "Syntax"
	LogCatgAPI        = "API"
	LogCatgFS         = "Filesystem"
	LogCatgCompletion = "Completion"
	LogCatgPlumb      = "Plumbing"
	LogCatgWin        = "Window"
	LogCatgCmd        = "Commands"
	LogCatgCol        = "Column"
	LogCatgConf       = "Config"
	LogCatgEditor     = "Editor"
	LogCatgPack       = "Packing"
	LogCatgSsh        = "SSH"
	LogCatgExpr       = "Expressions"
)

var debugLogCategories = []string{
	LogCatgApp,
	LogCatgUI,
	LogCatgEd,
	LogCatgSyntax,
	LogCatgAPI,
	LogCatgFS,
	LogCatgCompletion,
	LogCatgPlumb,
	LogCatgWin,
	LogCatgCmd,
	LogCatgCol,
	LogCatgConf,
	LogCatgEditor,
	LogCatgPack,
	LogCatgSsh,
	LogCatgExpr,
}

func startPprofDebugServer() {
	go func() {
		mylog.Check(http.ListenAndServe("localhost:6060", nil))
	}()
}
