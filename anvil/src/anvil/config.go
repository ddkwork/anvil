package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"github.com/ddkwork/golibrary/mylog"
	"github.com/ddkwork/golibrary/stream"
	"github.com/flopp/go-findfont"
	"github.com/pelletier/go-toml"

	"github.com/jeffwilliams/anvil/internal/typeset"
)

var ConfDir string

func init() {
	if runtime.GOOS == "windows" {
		ConfDir = fmt.Sprintf("%s/.anvil", os.Getenv("USERPROFILE"))
	} else {
		ConfDir = fmt.Sprintf("%s/.anvil", os.Getenv("HOME"))
	}
}

func SshKeyDir() string {
	return fmt.Sprintf("%s/%s", ConfDir, "sshkeys")
}

func LoadSshKeys() {
	d := SshKeyDir()
	entries := mylog.Check2(os.ReadDir(d))

	for _, e := range entries {
		log(LogCatgConf, "Loading ssh key %s\n", e.Name())
		path := filepath.Join(d, e.Name())
		sshClientCache.AddKeyFromFile(e.Name(), path)
	}
}

func StyleConfigFile() string {
	return fmt.Sprintf("%s/%s", ConfDir, "style.js")
}

func loadFontFromFile(filename string) (f text.FontFace, err error) {
	path := mylog.Check2(findfont.Find(filename))

	file := mylog.Check2(os.Open(path))

	defer func() { mylog.Check(file.Close()) }()

	var face opentype.Face
	face = mylog.Check2(typeset.ParseTTF(file))

	return fontFaceFromOpentype(face, filepath.Base(filename)), nil
}

func fontFaceFromOpentype(face opentype.Face, typefaceName string) text.FontFace {
	ff := text.FontFace{
		Font: font.Font{
			Typeface: font.Typeface(typefaceName),
		},
		Face: face,
	}

	return ff
}

func LoadStyleFromConfigFile(defaults *Style) (s Style, err error) {
	return LoadStyleFromFile(StyleConfigFile(), defaults)
}

func LoadStyleFromFile(path string, defaults *Style) (s Style, err error) {
	s = mylog.Check2(ReadStyle(path, defaults))

	for i, f := range s.Fonts {
		s.Fonts[i].FontFace = VariableFont
		if f.FontName != "" {

			if f.FontName == "defaultMonoFont" {
				s.Fonts[i].FontFace = MonoFont
				continue
			}

			if f.FontName == "defaultVariableFont" {
				s.Fonts[i].FontFace = VariableFont
				continue
			}

			var fnt text.FontFace
			fnt = mylog.Check2(loadFontFromFile(f.FontName))
			mylog.Check(err)

			s.Fonts[i].FontFace = fnt
		}
	}

	return
}

func LoadCurrentStyleFromFile(path string, defaults *Style) (err error) {
	s := mylog.Check2(LoadStyleFromFile(path, defaults))

	WindowStyle = s
	editor.SetStyle(WindowStyle)
	return
}

func SaveCurrentStyleToFile(path string) (err error) {
	mylog.Check(WriteStyle(path, WindowStyle))
	return
}

func PlumbingConfigFile() string {
	return fmt.Sprintf("%s/%s", ConfDir, "plumbing")
}

func LoadPlumbingRulesFromFile(path string) (rules []PlumbingRule) {
	stream.WriteTruncate(path, "") // todo

	f := mylog.Check2(os.Open(path))
	rules = ParsePlumbingRules(f)
	defer func() { mylog.Check(f.Close()) }()
	mylog.CheckNil(rules)
	return
}

func SettingsConfigFile() string {
	return fmt.Sprintf("%s/%s", ConfDir, "settings.toml")
}

type Settings struct {
	Ssh         SshSettings
	Typesetting TypesettingSettings
	Layout      LayoutSettings
}

type SshSettings struct {
	Shell      string
	CloseStdin bool `toml:"close-stdin"`
	CacheSize  int
	Env        map[string]string
}

type TypesettingSettings struct {
	ReplaceCRWithTofu bool `toml:"replace-cr-with-tofu"`
}

func LoadSettingsFromConfigFile(settings *Settings) (err error) {
	var f *os.File
	f = mylog.Check2(os.Open(SettingsConfigFile()))

	defer func() { mylog.Check(f.Close()) }()

	dec := toml.NewDecoder(f)
	mylog.Check(dec.Decode(settings))
	return
}

type LayoutSettings struct {
	EditorTag         string `toml:"editor-tag"`
	ColumnTag         string `toml:"column-tag"`
	WindowTagUserArea string `toml:"window-tag-user-area"`
}

func GenerateSampleSettings() string {
	return `# Sample anvil settings file
[layout]
# The default part of the editor tag that does not include running commands
#editor-tag="Newcol Kill Putall Dump Load Exit Help ◊ "

# The default column tag
#column-tag="New Cut Paste Snarf Zerox Delcol "

# The default part of the window tag that the user can edit
#window-tag-user-area=" Do Look "

[typesetting]
# When rendering text show carriage-returns as the "tofu" character (a box)
# The default is false
#replace-cr-with-tofu=false

[ssh]
# shell specifies the shell to use when commands are executed on a remote system.
# The default is "sh"
#shell="sh"

# close-stdin controls if stdin is closed for remote programs when they are executed
# using middle-click. Some programs require this to operate properly such as ripgrep,
# while some require stdin to be open even if it is not read, like man or git.
# The default is false
#close-stdin=false

# cachesize is the max number of ssh sessions kept open at once. Each user, host, port, proxy
# combination requires a different connection
#cachesize=5

# The ssh.env table lists environment variables to be exported when running remote
# commands.
#[ssh.env]
#VAR="val"



`
}
