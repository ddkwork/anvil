package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"strings"

	"github.com/ddkwork/golibrary/mylog"
	"golang.org/x/image/colornames"

	"gioui.org/text"
)

type Style struct {
	Fonts                     []FontStyle
	TagFgColor                Color
	TagBgColor                Color
	TagPathBasenameColor      Color
	BodyFgColor               Color
	BodyBgColor               Color
	LayoutBoxFgColor          Color
	LayoutBoxUnsavedBgColor   Color
	LayoutBoxBgColor          Color
	ScrollFgColor             Color
	ScrollBgColor             Color
	GutterWidth               int
	WinBorderColor            Color
	WinBorderWidth            int
	PrimarySelectionFgColor   Color
	PrimarySelectionBgColor   Color
	ExecutionSelectionFgColor Color
	ExecutionSelectionBgColor Color
	SecondarySelectionFgColor Color
	SecondarySelectionBgColor Color
	ErrorsTagFgColor          Color
	ErrorsTagBgColor          Color
	ErrorsTagFlashFgColor     Color
	ErrorsTagFlashBgColor     Color
	TabStopInterval           int
	Syntax                    SyntaxStyle
	Ansi                      AnsiStyle
	LineSpacing               int
	TextLeftPadding           int
}

type FontStyle struct {
	FontName string
	FontSize int
	FontFace text.FontFace `json:"-"` // Don't write the Font property to file.
}

type SyntaxStyle struct {
	KeywordColor      Color
	NameColor         Color
	StringColor       Color
	NumberColor       Color
	OperatorColor     Color
	CommentColor      Color
	PreprocessorColor Color
	HeadingColor      Color
	SubheadingColor   Color
	InsertedColor     Color
	DeletedColor      Color
}

type AnsiStyle struct {
	Colors [16]Color
}

func (as AnsiStyle) AsColors() [16]color.NRGBA {
	var c [16]color.NRGBA
	for i, v := range as.Colors {
		c[i] = color.NRGBA(v)
	}
	return c
}

func (s Style) tagEditableStyle() editableStyle {
	return editableStyle{
		Fonts:       s.Fonts,
		FgColor:     s.TagFgColor,
		LineSpacing: s.LineSpacing,
		PrimarySelection: textStyle{
			FgColor: s.PrimarySelectionFgColor,
			BgColor: s.PrimarySelectionBgColor,
		},
		SecondarySelection: textStyle{
			FgColor: s.SecondarySelectionFgColor,
			BgColor: s.SecondarySelectionBgColor,
		},
		ExecutionSelection: textStyle{
			FgColor: s.ExecutionSelectionFgColor,
			BgColor: s.ExecutionSelectionBgColor,
		},
		TabStopInterval: s.TabStopInterval,
		TextLeftPadding: s.TextLeftPadding,
	}
}

func (s Style) tagBlockStyle() blockStyle {
	return blockStyle{
		StandardBgColor:   color.NRGBA(s.TagBgColor),
		ErrorBgColor:      color.NRGBA(s.ErrorsTagBgColor),
		ErrorFlashBgColor: color.NRGBA(s.ErrorsTagFlashBgColor),
		PathBasenameColor: color.NRGBA(s.TagPathBasenameColor),
	}
}

func (s Style) bodyBlockStyle() blockStyle {
	return blockStyle{
		StandardBgColor: color.NRGBA(s.BodyBgColor),
	}
}

func (s Style) bodyEditableStyle() editableStyle {
	return editableStyle{
		Fonts:       s.Fonts,
		FgColor:     s.BodyFgColor,
		LineSpacing: s.LineSpacing,
		PrimarySelection: textStyle{
			FgColor: s.PrimarySelectionFgColor,
			BgColor: s.PrimarySelectionBgColor,
		},
		SecondarySelection: textStyle{
			FgColor: s.SecondarySelectionFgColor,
			BgColor: s.SecondarySelectionBgColor,
		},
		ExecutionSelection: textStyle{
			FgColor: s.ExecutionSelectionFgColor,
			BgColor: s.ExecutionSelectionBgColor,
		},
		TabStopInterval: s.TabStopInterval,
		TextLeftPadding: s.TextLeftPadding,
	}
}

func (s Style) layoutBoxStyle() layoutBoxStyle {
	return layoutBoxStyle{
		FgColor:        color.NRGBA(s.LayoutBoxFgColor),
		UnsavedBgColor: color.NRGBA(s.LayoutBoxUnsavedBgColor),
		BgColor:        color.NRGBA(s.LayoutBoxBgColor),
		GutterWidth:    s.GutterWidth,
	}
}

func (s Style) scrollbarStyle() scrollbarStyle {
	return scrollbarStyle{
		FgColor:     color.NRGBA(s.ScrollFgColor),
		BgColor:     color.NRGBA(s.ScrollBgColor),
		GutterWidth: s.GutterWidth,
	}
}

func MustParseHexColor(s string) (c Color) {
	return mylog.Check2(ParseHexColor(s))
}

func ParseHexColor(s string) (c Color, err error) {
	c.A = 0xff

	if s[0] != '#' {
		mylog.Check(fmt.Errorf("Invalid hex color format when parsing '%s': does not begin with #", s))
		return
	}

	hexToByte := func(b byte) byte {
		switch {
		case b >= '0' && b <= '9':
			return b - '0'
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10
		}
		mylog.Check(fmt.Errorf("Invalid hex color format when parsing '%s': contains a character that is not 0-9, a-f or A-F", s))
		return 0
	}

	switch len(s) {
	case 7:
		c.R = hexToByte(s[1])<<4 + hexToByte(s[2])
		c.G = hexToByte(s[3])<<4 + hexToByte(s[4])
		c.B = hexToByte(s[5])<<4 + hexToByte(s[6])
	case 4:
		c.R = hexToByte(s[1]) * 17
		c.G = hexToByte(s[2]) * 17
		c.B = hexToByte(s[3]) * 17
	default:
		mylog.Check(fmt.Errorf("Invalid hex color format when parsing '%s': length is not 4 or 7 bytes", s))
		return
	}
	return
}

func ReadStyle(path string, defaults *Style) (s Style, err error) {
	if defaults != nil {
		s = *defaults
	}
	file := mylog.Check2(os.Open(path))
	defer file.Close()

	enc := json.NewDecoder(file)
	mylog.Check(enc.Decode(&s))
	return
}

// WriteStyle writes the style to a file.
// Note that we omit marshalling the Font property because it is pretty big. However it would be interesting
// to be able to export the font to the file, modify it by hand and import it again.
func WriteStyle(path string, s Style) error {
	file := mylog.Check2(os.Create(path))

	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}

type Color color.NRGBA

func (c Color) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"#%02x%02x%02x\"", c.R, c.G, c.B)), nil
}

func (c *Color) UnmarshalJSON(b []byte) error {
	s := string(b)
	if b[0] != '"' || b[len(s)-1] != '"' {
		return fmt.Errorf("Invalid hex color format when unmarshalling JSON color '%s': color should be a string value (in double-quotes)", s)
	}
	col := mylog.Check2(ParseHexColor(string(b[1 : len(b)-1])))

	*c = col
	return nil
}

func ColorFromName(name string) (c Color, ok bool) {
	name = strings.ToLower(name)

	col, ok := colornames.Map[name]
	if !ok {
		return
	}
	return Color{R: col.R, G: col.G, B: col.B, A: col.A}, true
}
