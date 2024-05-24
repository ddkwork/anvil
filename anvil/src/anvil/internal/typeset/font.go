package typeset

import (
	"io/ioutil"

	"gioui.org/font/opentype"
	"github.com/ddkwork/golibrary/mylog"
)

func ParseTTFBytes(b []byte) (opentype.Face, error) {
	return opentype.Parse(b)
}

func ParseTTF(r Resource) (opentype.Face, error) {
	b := mylog.Check2(ioutil.ReadAll(r))

	return ParseTTFBytes(b)
}

type Resource interface {
	Read([]byte) (int, error)
	ReadAt([]byte, int64) (int, error)
	Seek(int64, int) (int64, error)
}
