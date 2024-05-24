package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/ddkwork/golibrary/mylog"
	"golang.org/x/sys/unix"
)

func startCmd(argv []string) (stdin io.Writer, stdout io.Reader, terminated func() bool, err error) {
	// fmt.Printf("Running command %s %s\n", os.Args[1], strings.Join(args, " "))

	c := exec.Command(argv[0], argv[1:]...)

	tty := mylog.Check2(pty.Start(c))
	setNoEcho(tty)

	stdin = tty
	stdout = tty

	ch := make(chan struct{})
	go func() {
		c.Process.Wait()
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

func setNoEcho(tty *os.File) {
	fd := int(tty.Fd())

	termios := mylog.Check2(unix.IoctlGetTermios(fd, unix.TCGETS))

	newState := *termios
	newState.Lflag &^= unix.ECHO
	newState.Lflag |= unix.ICANON | unix.ISIG
	newState.Iflag |= unix.ICRNL
	mylog.Check(unix.IoctlSetTermios(fd, unix.TCSETS, &newState))
}
