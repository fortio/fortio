// Copyright 2018 Istio Authors
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

// Package bincommon is the common code and flag handling between the fortio
// (fortio_main.go) and fcurl (fcurl.go) executables.
package bincommon

// Do not add any external dependencies we want to keep fortio minimal.

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"

	"fortio.org/fortio/dflag"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/log"
	"fortio.org/fortio/version"
)

// -- Support for multiple instances of -H flag on cmd line.
type headersFlagList struct{}

func (f *headersFlagList) String() string {
	return ""
}

func (f *headersFlagList) Set(value string) error {
	return httpOpts.AddAndValidateExtraHeader(value)
}

// -- end of functions for -H support

// FlagsUsage prints end of the usage() (flags part + error message).
func FlagsUsage(w io.Writer, msgs ...interface{}) {
	_, _ = fmt.Fprintf(w, "flags are:\n")
	flag.CommandLine.SetOutput(w)
	flag.PrintDefaults()
	if len(msgs) > 0 {
		_, _ = fmt.Fprintln(w, msgs...)
	}
}

var (
	compressionFlag = flag.Bool("compression", false, "Enable http compression")
	keepAliveFlag   = flag.Bool("keepalive", true, "Keep connection alive (only for fast http 1.1)")
	halfCloseFlag   = flag.Bool("halfclose", false,
		"When not keepalive, whether to half close the connection (only for fast http)")
	httpReqTimeoutFlag  = flag.Duration("timeout", fhttp.HTTPReqTimeOutDefaultValue, "Connection and read timeout value (for http)")
	stdClientFlag       = flag.Bool("stdclient", false, "Use the slower net/http standard client (slower but supports h2)")
	http10Flag          = flag.Bool("http1.0", false, "Use http1.0 (instead of http 1.1)")
	httpsInsecureFlag   = flag.Bool("k", false, "Do not verify certs in https/tls/grpc connections")
	httpsInsecureFlagL  = flag.Bool("https-insecure", false, "Long form of the -k flag")
	resolve             = flag.String("resolve", "", "Resolve host name to this `IP`")
	headersFlags        headersFlagList
	httpOpts            fhttp.HTTPOptions
	followRedirectsFlag = flag.Bool("L", false, "Follow redirects (implies -std-client) - do not use for load test")
	userCredentialsFlag = flag.String("user", "", "User credentials for basic authentication (for http). Input data format"+
		" should be `user:password`")
	// QuietFlag is the value of -quiet.
	QuietFlag       = flag.Bool("quiet", false, "Quiet mode: sets the loglevel to Error and reduces the output.")
	contentTypeFlag = flag.String("content-type", "",
		"Sets http content type. Setting this value switches the request method from GET to POST.")
	// PayloadSizeFlag is the value of -payload-size.
	PayloadSizeFlag = flag.Int("payload-size", 0, "Additional random payload size, replaces -payload when set > 0,"+
		" must be smaller than -maxpayloadsizekb. Setting this switches http to POST.")
	// PayloadFlag is the value of -payload.
	PayloadFlag = flag.String("payload", "", "Payload string to send along")
	// PayloadFileFlag is the value of -paylaod-file.
	PayloadFileFlag = flag.String("payload-file", "", "File `path` to be use as payload (POST for http), replaces -payload when set.")
	// UnixDomainSocket to use instead of regular host:port.
	unixDomainSocketFlag = flag.String("unix-socket", "", "Unix domain socket `path` to use for physical connection")
	// ConfigDirectoryFlag is where to watch for dynamic flag updates.
	ConfigDirectoryFlag = flag.String("config", "",
		"Config directory `path` to watch for changes of dynamic flags (empty for no watch)")
	// CertFlag is the flag for the path for the client custom certificate.
	CertFlag = flag.String("cert", "", "`Path` to the certificate file to be used for client or server TLS")
	// KeyFlag is the flag for the path for the key for the `cert`.
	KeyFlag = flag.String("key", "", "`Path` to the key file matching the -cert")
	// CACertFlag is the flag for the path of the custom CA to verify server certificates in client calls.
	CACertFlag = flag.String("cacert", "",
		"`Path` to a custom CA certificate file to be used for the TLS client connections, "+
			"if empty, use https:// prefix for standard internet/system CAs")
	// LogErrorsFlag determines if the non ok http error codes get logged as they occur or not.
	LogErrorsFlag = flag.Bool("log-errors", true, "Log http non 2xx/418 error codes as they occur")
	// RunIDFlag is optional RunID to be present in json results (and default json result filename if not 0).
	RunIDFlag = flag.Int64("runid", 0, "Optional RunID to add to json result and auto save filename, to match server mode")
	// HelpFlag is true if help/usage is being requested by the user.
	HelpFlag   = flag.Bool("h", false, "Print usage/help on stdout")
	warmupFlag = flag.Bool("sequential-warmup", false,
		"http(s) runner warmup done in parallel instead of sequentially. When set, restores pre 1.21 behavior")
	curlHeadersStdout = flag.Bool("curl-stdout-headers", false,
		"Restore pre 1.22 behavior where http headers of the fast client are output to stdout in curl mode. now stderr by default.")
	// MaxConnectionReuse Dynamic string flag to set the max connection reuse range.
	connectionReuseRange = dflag.DynString(flag.CommandLine, "connection-reuse-range", "",
		"Range `min:max` for the max number of connections to reuse for each thread, default to unlimited. "+
			"e.g. 10:30 means randomly choose a max connection reuse threshold between 10 and 30 requests.").
		WithValidator(maxConnectionReuseValidator)
)

// SharedMain is the common part of main from fortio_main and fcurl.
func SharedMain(usage func(io.Writer, ...interface{})) {
	flag.Var(&headersFlags, "H", "Additional `header`(s)")
	flag.IntVar(&fhttp.BufferSizeKb, "httpbufferkb", fhttp.BufferSizeKb,
		"Size of the buffer (max data size) for the optimized http client in `kbytes`")
	flag.BoolVar(&fhttp.CheckConnectionClosedHeader, "httpccch", fhttp.CheckConnectionClosedHeader,
		"Check for Connection: Close Header")
	// Special case so `fcurl -version` and `--version` and `version` and ... work
	if len(os.Args) < 2 {
		return
	}
	firstArg := os.Args[1]
	if strings.Contains(firstArg, "version") {
		if len(os.Args) >= 3 && strings.Contains(os.Args[2], "s") {
			// so `fortio version -s` is the short version; everything else is long/full
			fmt.Println(version.Short())
		} else {
			fmt.Print(version.Full())
		}
		os.Exit(0)
	}
	if strings.Contains(firstArg, "help") || firstArg == "-h" {
		usage(os.Stdout)
		os.Exit(0)
	}
}

// FetchURL is fetching url content and exiting with 1 upon error.
// common part between fortio_main and fcurl.
func FetchURL(o *fhttp.HTTPOptions) {
	// keepAlive could be just false when making 1 fetch but it helps debugging
	// the http client when making a single request if using the flags
	client, _ := fhttp.NewClient(o)
	// big gotcha that nil client isn't nil interface value (!)
	if client == nil || reflect.ValueOf(client).IsNil() {
		return // error logged already
	}
	code, data, header := client.Fetch()
	log.LogVf("Fetch result code %d, data len %d, headerlen %d", code, len(data), header)
	if *curlHeadersStdout {
		os.Stdout.Write(data)
	} else {
		os.Stderr.Write(data[:header])
		os.Stdout.Write(data[header:])
	}
	if code != http.StatusOK {
		log.Errf("Error status %d : %s", code, fhttp.DebugSummary(data, 512))
		os.Exit(1)
	}
}

// TLSInsecure returns true if -k or -https-insecure was passed.
func TLSInsecure() bool {
	TLSInsecure := *httpsInsecureFlag || *httpsInsecureFlagL
	if TLSInsecure {
		log.Infof("TLS certificates will not be verified, per flag request")
	} else {
		log.LogVf("Will verify TLS certificates, use -k / -https-insecure to disable")
	}
	return TLSInsecure
}

func maxConnectionReuseValidator(inp string) error {
	if inp == "" {
		return nil
	}

	reuseRangeString := strings.Split(inp, ":")
	var reuseRangeInt []int

	if len(reuseRangeString) > 2 {
		return fmt.Errorf("more than two integers were provided in the connection reuse range")
	}

	for _, input := range reuseRangeString {
		if val, err := strconv.Atoi(input); err != nil {
			return fmt.Errorf("invalid value for connection reuse range, err: %v", err)
		} else {
			reuseRangeInt = append(reuseRangeInt, val)
		}
	}

	if len(reuseRangeInt) == 1 {
		httpOpts.ConnReuseRange = [2]int{reuseRangeInt[0], reuseRangeInt[0]}
	} else {
		if reuseRangeInt[0] < reuseRangeInt[1] {
			httpOpts.ConnReuseRange = [2]int{reuseRangeInt[0], reuseRangeInt[1]}
		} else {
			httpOpts.ConnReuseRange = [2]int{reuseRangeInt[1], reuseRangeInt[0]}
		}
	}

	return nil
}

// SharedHTTPOptions is the flag->httpoptions transfer code shared between
// fortio_main and fcurl.
func SharedHTTPOptions() *fhttp.HTTPOptions {
	url := strings.TrimLeft(flag.Arg(0), " \t\r\n")
	httpOpts.URL = url
	httpOpts.HTTP10 = *http10Flag
	httpOpts.DisableFastClient = *stdClientFlag
	httpOpts.DisableKeepAlive = !*keepAliveFlag
	httpOpts.AllowHalfClose = *halfCloseFlag
	httpOpts.Compression = *compressionFlag
	httpOpts.HTTPReqTimeOut = *httpReqTimeoutFlag
	httpOpts.Insecure = TLSInsecure()
	httpOpts.Resolve = *resolve
	httpOpts.UserCredentials = *userCredentialsFlag
	httpOpts.ContentType = *contentTypeFlag
	httpOpts.Payload = fnet.GeneratePayload(*PayloadFileFlag, *PayloadSizeFlag, *PayloadFlag)
	httpOpts.UnixDomainSocket = *unixDomainSocketFlag
	if *followRedirectsFlag {
		httpOpts.FollowRedirects = true
		httpOpts.DisableFastClient = true
	}
	httpOpts.CACert = *CACertFlag
	httpOpts.Cert = *CertFlag
	httpOpts.Key = *KeyFlag
	httpOpts.LogErrors = *LogErrorsFlag
	httpOpts.SequentialWarmup = *warmupFlag
	return &httpOpts
}
