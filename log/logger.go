// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package log // import "fortio.org/fortio/log"

import (
	"flag"
	"fmt"
	"io"
	"log"
	"runtime"
	"strings"
	"sync/atomic"

	"fortio.org/fortio/dflag"
)

// Level is the level of logging (0 Debug -> 6 Fatal).
type Level int32

// Log levels. Go can't have variable and function of the same name so we keep
// medium length (Dbg,Info,Warn,Err,Crit,Fatal) names for the functions.
const (
	Debug Level = iota
	Verbose
	Info
	Warning
	Error
	Critical
	Fatal
)

var (
	levelToStrA []string
	levelToStrM map[string]Level
	// LogPrefix is a prefix to include in each log line.
	LogPrefix = flag.String("logprefix", "> ", "Prefix to log lines before logged messages")
	// LogFileAndLine determines if the log lines will contain caller file name and line number.
	LogFileAndLine = flag.Bool("logcaller", true, "Logs filename and line number of callers to log")
	levelInternal  int32
)

// SetFlagDefaultsForClientTools changes the default value of -logprefix and -logcaller
// to make output without caller and prefix, a default more suitable for command line tools (like dnsping).
// Needs to be called before flag.Parse(). Caller could also use log.Printf instead of changing this
// if not wanting to use levels.
func SetFlagDefaultsForClientTools() {
	lcf := flag.Lookup("logcaller")
	lcf.DefValue = "false"
	_ = lcf.Value.Set("false")
	lpf := flag.Lookup("logprefix")
	lpf.DefValue = ""
	_ = lpf.Value.Set("")
}

//nolint:gochecknoinits // needed
func init() {
	setLevel(Info) // starting value
	levelToStrA = []string{
		"Debug",
		"Verbose",
		"Info",
		"Warning",
		"Error",
		"Critical",
		"Fatal",
	}
	levelToStrM = make(map[string]Level, 2*len(levelToStrA))
	for l, name := range levelToStrA {
		// Allow both -loglevel Verbose and -loglevel verbose ...
		levelToStrM[name] = Level(l)
		levelToStrM[strings.ToLower(name)] = Level(l)
	}
	// virtual dynLevel flag that maps back to actual level
	_ = dflag.DynString(flag.CommandLine, "loglevel", GetLogLevel().String(),
		fmt.Sprintf("loglevel, one of %v", levelToStrA)).WithInputMutator(
		func(inp string) string {
			// The validation map has full lowercase and capitalized first letter version
			return strings.ToLower(strings.TrimSpace(inp))
		}).WithValidator(
		func(newStr string) error {
			_, err := ValidateLevel(newStr)
			return err
		}).WithSyncNotifier(
		func(old, newStr string) {
			_ = setLogLevelStr(newStr) // will succeed as we just validated it first
		})
	log.SetFlags(log.Ltime)
}

func setLevel(lvl Level) {
	atomic.StoreInt32(&levelInternal, int32(lvl))
}

// String returns the string representation of the level.
func (l Level) String() string {
	return levelToStrA[l]
}

// ValidateLevel returns error if the level string is not valid.
func ValidateLevel(str string) (Level, error) {
	var lvl Level
	var ok bool
	if lvl, ok = levelToStrM[str]; !ok {
		return -1, fmt.Errorf("should be one of %v", levelToStrA)
	}
	return lvl, nil
}

// Sets from string.
func setLogLevelStr(str string) error {
	var lvl Level
	var err error
	if lvl, err = ValidateLevel(str); err != nil {
		return err
	}
	SetLogLevel(lvl)
	return err // nil
}

// SetLogLevel sets the log level and returns the previous one.
func SetLogLevel(lvl Level) Level {
	return setLogLevel(lvl, true)
}

// SetLogLevelQuiet sets the log level and returns the previous one but does
// not log the change of level itself.
func SetLogLevelQuiet(lvl Level) Level {
	return setLogLevel(lvl, false)
}

// setLogLevel sets the log level and returns the previous one.
// if logChange is true the level change is logged.
func setLogLevel(lvl Level, logChange bool) Level {
	prev := GetLogLevel()
	if lvl < Debug {
		log.Printf("SetLogLevel called with level %d lower than Debug!", lvl)
		return -1
	}
	if lvl > Critical {
		log.Printf("SetLogLevel called with level %d higher than Critical!", lvl)
		return -1
	}
	if lvl != prev {
		if logChange {
			logPrintf(Info, "Log level is now %d %s (was %d %s)\n", lvl, lvl.String(), prev, prev.String())
		}
		setLevel(lvl)
	}
	return prev
}

// GetLogLevel returns the currently configured LogLevel.
func GetLogLevel() Level {
	return Level(atomic.LoadInt32(&levelInternal))
}

// Log returns true if a given level is currently logged.
func Log(lvl Level) bool {
	return int32(lvl) >= atomic.LoadInt32(&levelInternal)
}

// LevelByName returns the LogLevel by its name.
func LevelByName(str string) Level {
	return levelToStrM[str]
}

// Logf logs with format at the given level.
// 2 level of calls so it's always same depth for extracting caller file/line.
func Logf(lvl Level, format string, rest ...interface{}) {
	logPrintf(lvl, format, rest...)
}

func logPrintf(lvl Level, format string, rest ...interface{}) {
	if !Log(lvl) {
		return
	}
	if *LogFileAndLine {
		_, file, line, _ := runtime.Caller(2)
		file = file[strings.LastIndex(file, "/")+1:]
		log.Print(levelToStrA[lvl][0:1], " ", file, ":", line, *LogPrefix, fmt.Sprintf(format, rest...))
	} else {
		log.Print(levelToStrA[lvl][0:1], " ", *LogPrefix, fmt.Sprintf(format, rest...))
	}
	if lvl == Fatal {
		panic("aborting...")
	}
}

// Printf forwards to the underlying go logger to print (with only timestamp prefixing).
func Printf(format string, rest ...interface{}) {
	log.Printf(format, rest...)
}

// SetOutput sets the output to a different writer (forwards to system logger).
func SetOutput(w io.Writer) {
	log.SetOutput(w)
}

// SetFlags forwards flags to the system logger.
func SetFlags(f int) {
	log.SetFlags(f)
}

// -- would be nice to be able to create those in a loop instead of copypasta:

// Debugf logs if Debug level is on.
func Debugf(format string, rest ...interface{}) {
	logPrintf(Debug, format, rest...)
}

// LogVf logs if Verbose level is on.
func LogVf(format string, rest ...interface{}) { //nolint:revive
	logPrintf(Verbose, format, rest...)
}

// Infof logs if Info level is on.
func Infof(format string, rest ...interface{}) {
	logPrintf(Info, format, rest...)
}

// Warnf logs if Warning level is on.
func Warnf(format string, rest ...interface{}) {
	logPrintf(Warning, format, rest...)
}

// Errf logs if Warning level is on.
func Errf(format string, rest ...interface{}) {
	logPrintf(Error, format, rest...)
}

// Critf logs if Warning level is on.
func Critf(format string, rest ...interface{}) {
	logPrintf(Critical, format, rest...)
}

// Fatalf logs if Warning level is on.
func Fatalf(format string, rest ...interface{}) {
	logPrintf(Fatal, format, rest...)
}

// LogDebug shortcut for fortio.Log(fortio.Debug).
func LogDebug() bool { //nolint:revive
	return Log(Debug)
}

// LogVerbose shortcut for fortio.Log(fortio.Verbose).
func LogVerbose() bool { //nolint:revive
	return Log(Verbose)
}

// LoggerI defines a log.Logger like interface for simple logging.
type LoggerI interface {
	Printf(format string, rest ...interface{})
}

type loggerShm struct{}

func (l *loggerShm) Printf(format string, rest ...interface{}) {
	logPrintf(Info, format, rest...)
}

// Logger returns a LoggerI (standard logger compatible) that can be used for simple logging.
func Logger() LoggerI {
	logger := loggerShm{}
	return &logger
}
