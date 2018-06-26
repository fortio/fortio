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
	"net/http"
	"os"
	"strings"

	"io"

	"istio.io/fortio/fhttp"
	"istio.io/fortio/log"
	"istio.io/fortio/version"
)

// -- Support for multiple instances of -H flag on cmd line:
type headersFlagList struct {
}

func (f *headersFlagList) String() string {
	return ""
}
func (f *headersFlagList) Set(value string) error {
	return httpOpts.AddAndValidateExtraHeader(value)
}

// -- end of functions for -H support

// FlagsUsage prints end of the usage() (flags part + error message).
func FlagsUsage(writer io.Writer, msgs ...interface{}) {
	// nolint: gas
	fmt.Fprintf(writer, "flags are:\n")
	flag.CommandLine.SetOutput(writer)
	flag.PrintDefaults()
	if len(msgs) > 0 {
		fmt.Fprint(writer, msgs...) // nolint: gas
	}
}

// Usage prints usage according to input writer
func Usage(writer io.Writer) {
	fmt.Fprintf(writer, "Φορτίο %s usage:\n\t%s command [flags] target\n%s\n%s\n%s\n%s\n",
		version.Short(),
		os.Args[0],
		"where command is one of: load (load testing), server (starts grpc ping and",
		"http echo/ui/redirect/proxy servers), grpcping (grpc client), report (report only UI",
		"server), redirect (redirect only server), or curl (single URL debug).",
		"where target is a url (http load tests) or host:port (grpc health test).")
}

var (
	compressionFlag = flag.Bool("compression", false, "Enable http compression")
	keepAliveFlag   = flag.Bool("keepalive", true, "Keep connection alive (only for fast http 1.1)")
	halfCloseFlag   = flag.Bool("halfclose", false,
		"When not keepalive, whether to half close the connection (only for fast http)")
	httpReqTimeoutFlag  = flag.Duration("timeout", fhttp.HTTPReqTimeOutDefaultValue, "Connection and read timeout value (for http)")
	stdClientFlag       = flag.Bool("stdclient", false, "Use the slower net/http standard client (works for TLS)")
	http10Flag          = flag.Bool("http1.0", false, "Use http1.0 (instead of http 1.1)")
	httpsInsecureFlag   = flag.Bool("k", false, "Do not verify certs in https connections")
	headersFlags        headersFlagList
	httpOpts            fhttp.HTTPOptions
	followRedirectsFlag = flag.Bool("L", false, "Follow redirects (implies -std-client) - do not use for load test")
	userCredentialsFlag = flag.String("user", "", "User credentials for basic authentication (for http). Input data format"+
		" should be user:password")
	// QuietFlag is the value of -quiet.
	QuietFlag = flag.Bool("quiet", false, "Quiet mode: sets the loglevel to Error and reduces the output.")
)

// SharedMain is the common part of main from fortio_main and fcurl.
func SharedMain() {
	flag.Var(&headersFlags, "H", "Additional Header(s)")
	flag.IntVar(&fhttp.BufferSizeKb, "httpbufferkb", fhttp.BufferSizeKb,
		"Size of the buffer (max data size) for the optimized http client in kbytes")
	flag.BoolVar(&fhttp.CheckConnectionClosedHeader, "httpccch", fhttp.CheckConnectionClosedHeader,
		"Check for Connection: Close Header")
	// Special case so `fcurl -version` and `--version` and `version` and ... work
	if len(os.Args) < 2 {
		return
	}
	if strings.Contains(os.Args[1], "version") {
		if len(os.Args) >= 3 && strings.Contains(os.Args[2], "s") {
			// so `fortio version -s` is the short version; everything else is long/full
			fmt.Println(version.Short())
		} else {
			fmt.Println(version.Long())
		}
		os.Exit(0)
	}
	if strings.Contains(os.Args[0], "fortio") && strings.Contains(os.Args[1], "help") {
		Usage(os.Stdout)
		FlagsUsage(os.Stdout)
		os.Exit(0)
	}
}

// FetchURL is fetching url content and exiting with 1 upon error.
// common part between fortio_main and fcurl.
func FetchURL(o *fhttp.HTTPOptions) {
	// keepAlive could be just false when making 1 fetch but it helps debugging
	// the http client when making a single request if using the flags
	client := fhttp.NewClient(o)
	if client == nil {
		return // error logged already
	}
	code, data, header := client.Fetch()
	log.LogVf("Fetch result code %d, data len %d, headerlen %d", code, len(data), header)
	os.Stdout.Write(data) //nolint: errcheck
	if code != http.StatusOK {
		log.Errf("Error status %d : %s", code, fhttp.DebugSummary(data, 512))
		os.Exit(1)
	}
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
	httpOpts.Insecure = *httpsInsecureFlag
	httpOpts.UserCredentials = *userCredentialsFlag
	if *followRedirectsFlag {
		httpOpts.FollowRedirects = true
		httpOpts.DisableFastClient = true
	}
	return &httpOpts
}
