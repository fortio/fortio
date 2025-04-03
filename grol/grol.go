// Interface with the GROL scripting engine
package grol

import (
	"flag"
	"io"
	"os"
	"path/filepath"

	"fortio.org/log"
	"grol.io/grol/eval"
	"grol.io/grol/extensions"
	"grol.io/grol/repl"
)

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
