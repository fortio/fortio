// Interface with the GROL scripting engine.
package grol

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
	"fortio.org/log"
	"grol.io/grol/eval"
	"grol.io/grol/extensions"
	"grol.io/grol/object"
	"grol.io/grol/repl"
)

func createFortioGrolFunctions() {
	fn := object.Extension{
		Name:     "fortio.load",
		MinArgs:  2,
		MaxArgs:  2,
		Help:     "Start a load test of given type (http, tcp, udp, grpc) with the passed in map/json parameters (url, qps, etc)",
		ArgTypes: []object.Type{object.STRING, object.MAP},
		Callback: func(env any, _ string, args []object.Object) object.Object {
			s := env.(*eval.State)
			runType := args[0].(object.String).Value
			// to JSON and then back to RunnerOptions
			w := strings.Builder{}
			err := args[1].JSON(&w)
			if err != nil {
				return s.Error(err)
			}
			// Use http as the base/most common - it has everything we need and we can transfer the URL into
			// Destination for other types.
			ro := fhttp.HTTPRunnerOptions{}
			err = json.Unmarshal([]byte(w.String()), &ro)
			if err != nil {
				return s.Error(err)
			}
			// Restore terminal to normal mode while the runner runs so ^C is handled by the regular fortio aborter code.
			if s.Term != nil {
				s.Term.Suspend()
			}
			//nolint:fatcontext // we do need to update/reset the context and its cancel function.
			s.Context, s.Cancel = context.WithCancel(context.Background()) // no timeout.
			log.LogVf("Running %s %#v", runType, ro)
			var res any
			switch runType {
			case "http":
				res, err = fhttp.RunHTTPTest(&ro)
			case "tcp":
				tro := tcprunner.RunnerOptions{
					RunnerOptions: ro.RunnerOptions,
				}
				tro.Destination = ro.URL
				res, err = tcprunner.RunTCPTest(&tro)
			case "udp":
				uro := udprunner.RunnerOptions{
					RunnerOptions: ro.RunnerOptions,
				}
				uro.Destination = ro.URL
				res, err = udprunner.RunUDPTest(&uro)
			case "grpc":
				gro := fgrpc.GRPCRunnerOptions{}
				// re deserialize that one as grpc has unique options.
				err = json.Unmarshal([]byte(w.String()), &gro)
				if err != nil {
					return s.Error(err)
				}
				if gro.Destination == "" {
					gro.Destination = ro.URL
				}
				res, err = fgrpc.RunGRPCTest(&gro)
			default:
				return s.Errorf("Run type %q unexpected", runType)
			}
			// Put it back to grol mode when done. alternative is have ro.Out = s.Out and carry cancel function to runner's.
			if s.Term != nil {
				s.Context, s.Cancel = s.Term.Resume(context.Background())
			}
			if err != nil {
				return s.Error(err)
			}
			jsonData, jerr := json.Marshal(res)
			if jerr != nil {
				return s.Error(jerr)
			}
			// This is basically "unjson" implementation.
			obj, err := eval.EvalString(s, string(jsonData), true)
			if err != nil {
				return s.Error(err)
			}
			return obj
		},
		DontCache: true,
	}
	extensions.MustCreate(fn)
}

func ScriptMode() int {
	// we already have either 0 or exactly 1 argument from the flag parsing.
	interactive := len(flag.Args()) == 0
	options := repl.Options{
		ShowEval: true,
	}
	// TODO: Carry some flags from the grol binary rather than hardcoded "safe"-ish config here.
	c := extensions.Config{
		HasLoad:           true,
		HasSave:           interactive,
		UnrestrictedIOs:   interactive,
		LoadSaveEmptyOnly: false,
	}
	err := extensions.Init(&c)
	if err != nil {
		return log.FErrf("Error initializing extensions: %v", err)
	}
	createFortioGrolFunctions()
	if interactive {
		// Maybe move some of the logic to grol package? (it's copied from grol's main for now)
		homeDir, err := os.UserHomeDir()
		histFile := filepath.Join(homeDir, ".fortio_history")
		if err != nil {
			log.Warnf("Couldn't get user home dir: %v", err)
			histFile = ""
		}
		options.HistoryFile = histFile
		options.MaxHistory = 99
		log.Printf("Starting interactive grol script mode")
		return repl.Interactive(options)
	}
	scriptFile := flag.Arg(0)
	var reader io.Reader = os.Stdin
	if scriptFile != "-" {
		f, err := os.Open(scriptFile)
		if err != nil {
			return log.FErrf("%v", err)
		}
		defer f.Close()
		reader = f
	}
	s := eval.NewState()
	errs := repl.EvalAll(s, reader, os.Stdout, options)
	if len(errs) > 0 {
		return log.FErrf("Errors: %v", errs)
	}
	return 0
}
