// Interface with the GROL scripting engine.
package grol

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fortio.org/fortio/fgrpc"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/periodic"
	"fortio.org/fortio/rapi"
	"fortio.org/fortio/tcprunner"
	"fortio.org/fortio/udprunner"
	"fortio.org/log"
	"grol.io/grol/eval"
	"grol.io/grol/extensions"
	"grol.io/grol/object"
	"grol.io/grol/repl"
)

// MapToStruct converts a grol map to a struct of type T by doing a JSON roundtrip.
func MapToStruct[T any](t *T, omap object.Map) error {
	w := strings.Builder{}
	err := omap.JSON(&w)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(w.String()), t)
	if err != nil {
		return err
	}
	return nil
}

func createFortioGrolFunctions(state *eval.State, scriptInit string) error {
	fn := object.Extension{
		Name:    "fortio.load",
		MinArgs: 2,
		MaxArgs: 2,
		Help: "Start a load test of given type (http, tcp, udp, grpc) with the passed in map/json parameters " +
			"(url, qps, etc, add \"save\":true to also save the result to a file)",
		ArgTypes:  []object.Type{object.STRING, object.MAP},
		Callback:  grolLoad,
		DontCache: true,
	}
	extensions.MustCreate(fn)
	fn.Name = "curl"
	fn.MinArgs = 1
	fn.MaxArgs = 2
	fn.Help = "fortio curl fetches the given url, with optional options"
	fn.Callback = grolCurl
	extensions.MustCreate(fn)
	// Shorter alias for http load test; can't use "load" as that's grol built-in for loading files.
	// Note we can't use eval.AddEvalResult() as we already made the state.
	_, err := eval.EvalString(state, "func hload(options){fortio.load(\"http\", options)}", false)
	if err != nil {
		panic(err)
	}
	// Add a conversion from seconds to durations int.
	_, err = eval.EvalString(state, "func duration(seconds){int(seconds * 1e9)}", false)
	if err != nil {
		panic(err)
	}
	// The above failure would be bug, thus the panic, while the below is a user error.
	if scriptInit != "" {
		obj, err := eval.EvalString(state, scriptInit, false)
		if err != nil {
			return fmt.Errorf("for %q: %w", scriptInit, err)
		}
		log.Infof("Script init %q: %v", scriptInit, obj.Inspect())
	}
	return nil
}

func grolLoad(env any, _ string, args []object.Object) object.Object {
	s := env.(*eval.State)
	runType := args[0].(object.String).Value
	// to JSON and then back to RunnerOptions
	omap := args[1].(object.Map)
	// Use http as the base/most common - it has everything we need and we can transfer the URL into
	// Destination for other types.
	ro := fhttp.HTTPRunnerOptions{}
	err := MapToStruct(&ro, omap)
	rapi.CallHook(&ro.HTTPOptions, &ro.RunnerOptions)
	if err != nil {
		return s.Error(err)
	}
	// Restore terminal to normal mode while the runner runs so ^C is handled by the regular fortio aborter code.
	if s.Term != nil {
		s.Term.Suspend()
	}
	s.Context, s.Cancel = context.WithCancel(context.Background()) // no timeout.
	log.LogVf("Running %s %#v", runType, ro)
	var res periodic.HasRunnerResult
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
		err = MapToStruct(&gro, omap)
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
	doSave, found := omap.Get(object.String{Value: "save"})
	if found && doSave == object.TRUE {
		fname := res.Result().ID + rapi.JSONExtension // third place we do this or similar...
		log.Infof("Saving %s", fname)
		err = os.WriteFile(fname, jsonData, 0o644) //nolint:gosec // we do want 644
		if err != nil {
			log.Errf("Unable to save %s: %v", fname, err)
			return s.Error(err)
		}
	}
	// This is basically "unjson"'s implementation.
	obj, err := eval.EvalString(s, string(jsonData), true)
	if err != nil {
		return s.Error(err)
	}
	return obj
}

func grolCurl(env any, _ string, args []object.Object) object.Object {
	s := env.(*eval.State)
	url := args[0].(object.String).Value
	httpOpts := fhttp.NewHTTPOptions(url)
	httpOpts.DisableFastClient = true
	httpOpts.FollowRedirects = true
	if len(args) > 1 {
		omap := args[1].(object.Map)
		err := MapToStruct(httpOpts, omap)
		if err != nil {
			return s.Error(err)
		}
	}
	var w bytes.Buffer
	httpOpts.DataWriter = &w
	rapi.CallHook(httpOpts, &periodic.RunnerOptions{})
	client, err := fhttp.NewClient(httpOpts)
	if err != nil {
		return s.Error(err)
	}
	code, _, _ := client.StreamFetch(context.Background())
	// must be pre-sorted!
	return object.MakeQuad(
		object.String{Value: "body"}, object.String{Value: w.String()},
		object.String{Value: "code"}, object.Integer{Value: int64(code)},
	)
}

func ScriptMode(scriptInit string) int {
	// we already have either 0 or exactly 1 argument from the flag parsing.
	interactive := len(flag.Args()) == 0
	options := repl.Options{
		ShowEval: true,
		// In interactive mode the state is created by that function, but there is a Hook so we use that so init script
		// can also set state even in interactive mode.
		PreInput: func(s *eval.State) {
			err := createFortioGrolFunctions(s, scriptInit)
			if err != nil {
				log.Errf("Error setting up initial scripting env: %v", err)
			}
		},
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
		log.SetDefaultsForClientTools()
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
