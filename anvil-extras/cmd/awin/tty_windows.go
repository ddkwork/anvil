package main

import (
	"context"
	"io"
	"strings"

	"github.com/UserExistsError/conpty"
	"github.com/ddkwork/golibrary/mylog"
)

func startCmd(argv []string) (stdin io.Writer, stdout io.Reader, terminated func() bool, err error) {
	c := strings.Join(argv, " ")
	debug("awin: running command '%s'\n", c)

	var tty *conpty.ConPty
	tty = mylog.Check2(conpty.Start(c))

	stdin = tty
	stdout = tty

	ch := make(chan struct{})
	go func() {
		// time.Sleep(1000 * time.Millisecond)
		code := mylog.Check2(tty.Wait(context.Background()))

		debug("awin: Wait returned with exit code: %d\n", code)
		close(ch)
	}()

	terminated = func() bool {
		select {
		case <-ch:
			return true
		default:
		}
		return false
	}

	return
}
