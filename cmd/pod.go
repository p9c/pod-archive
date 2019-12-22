package cmd

import (
	"fmt"
	// This enables pprof
	_ "net/http/pprof"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"runtime/trace"
	
	"github.com/p9c/pod/pkg/log"
	"github.com/p9c/pod/pkg/util/interrupt"
	
	"github.com/p9c/pod/app"
	"github.com/p9c/pod/pkg/util/limits"
)

var prevArgs []string

// Main is the main entry point for pod
func Main() {
	runtime.GOMAXPROCS(runtime.NumCPU() * 3)
	debug.SetGCPercent(10)
	if err := limits.SetLimits(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to set limits: %v\n", err)
		os.Exit(1)
	}
	if os.Getenv("POD_TRACE") == "on" {
		if f, err := os.Create("testtrace.out"); err != nil {
			log.ERROR("tracing env POD_TRACE=on but we can't write to it",
				err)
		} else {
			log.DEBUG("tracing started")
			err = trace.Start(f)
			if err != nil {
				log.ERROR("could not start tracing", err)
			} else {
				interrupt.AddHandler(
					func() {
						log.DEBUG("stopping trace")
						trace.Stop()
						err := f.Close()
						if err != nil {
							log.ERROR(err)
						}
					},
				)
			}
		}
	}
	interrupt.Reset = Reset(os.Args)
	app.Main()
}
func init() {
	prevArgs = os.Args
}

func Reset(newArgs []string) func() {
	return func() {
		var cmd *exec.Cmd
		if newArgs != nil {
			if prevArgs != nil {
				prevArgs = newArgs
			} else {
				prevArgs = os.Args
			}
		}
		cmd = exec.Command(prevArgs[0], prevArgs[1:]...)
		log.FATAL(cmd.Start())
		os.Exit(0)
	}
}
