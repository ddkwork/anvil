package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/ddkwork/golibrary/mylog"
)

// WindowsCmd builds an exec.Cmd that runs 'cmd.exe /C' with the passed arguments.
func WindowsCmd(arg string) *exec.Cmd {
	cmd := exec.Command("cmd")
	args := fmt.Sprintf("/C %s", arg)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    args,
	}
	return cmd
}

func KillProcess(p *os.Process) error {
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(p.Pid))
	mylog.Check(kill.Run())

	return p.Kill()
}
