package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	api "anvil-go-api"

	"github.com/ddkwork/golibrary/mylog"
	"github.com/ogier/pflag"
)

var (
	optInterval = pflag.IntP("interval", "i", 30, "Interval in seconds between dumps")
	optVerbose  = pflag.BoolP("verbose", "v", false, "Print extra information")
	optDumpfile = pflag.StringP("dumpfile", "f", "anvil-auto.dump", "Name of the dumpfile")
)

func main() {
	pflag.Parse()

	anvil := mylog.Check2(api.NewFromEnv())

	b := []byte(fmt.Sprintf(`{"cmd": "Dump", "args": ["%s"]}`, *optDumpfile))
	cmd := bytes.NewReader(b)

	interval := time.Duration(*optInterval) * time.Second
	if *optVerbose {
		fmt.Printf("autodump: started. Interval is %d seconds\n", *optInterval)
	}

	for {
		time.Sleep(interval)

		if *optVerbose {
			fmt.Printf("autodump: dumping\n")
		}
		_ := mylog.Check2(anvil.Post("/execute", cmd))

		cmd.Reset(b)
	}
}
