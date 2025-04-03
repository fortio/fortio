// Interface with the GROL scripting engine.
package grol

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fortio.org/fortio/fhttp"
	"fortio.org/log"
	"grol.io/grol/eval"
	"grol.io/grol/extensions"
	"grol.io/grol/object"
	"grol.io/grol/repl"
)

func createFortioGrolFunctions() {
	fn := object.Extension{
		Name:     "fortio.load",
		MinArgs:  1,
		MaxArgs:  1,
		Help:     "Start a load test with the passed in map/json parameters (url, qps, etc)",
		ArgTypes: []object.Type{object.MAP},
		Callback: func(env any, _ string, args []object.Object) object.Object {
			s := env.(*eval.State)
			// to JSON and then back to RunnerOptions
			w := strings.Builder{}
			err := args[0].JSON(&w)
			if err != nil {
				return s.Error(err)
			}
			ro := fhttp.HTTPRunnerOptions{}
			err = json.Unmarshal([]byte(w.String()), &ro)
			if err != nil {
				return s.Error(err)
			}
			ro.Out = s.Out
			log.Infof("Running %#v", ro)
			res, err := fhttp.RunHTTPTest(&ro)
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
