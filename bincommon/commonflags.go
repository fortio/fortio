// Copyright 2018 Fortio Authors
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
	"context"
	"flag"
	"net/http"
	"os"
	"reflect"
	"strings"

	"fortio.org/dflag"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/fnet"
	"fortio.org/fortio/periodic"
	"fortio.org/log"
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

// FortioHook is used in cli and rapi to customize the run and introduce for instance clienttrace
// and otel access logger.
type FortioHook func(*fhttp.HTTPOptions, *periodic.RunnerOptions)

var (
	compressionFlag = flag.Bool("compression", false, "Enable http compression")
	keepAliveFlag   = flag.Bool("keepalive", true, "Keep connection alive (only for fast http 1.1)")
	halfCloseFlag   = flag.Bool("halfclose", false,
		"When not keepalive, whether to half close the connection (only for fast http)")
	httpReqTimeoutFlag  = flag.Duration("timeout", fhttp.HTTPReqTimeOutDefaultValue, "Connection and read timeout value (for http)")
	stdClientFlag       = flag.Bool("stdclient", false, "Use the slower net/http standard client (slower but supports h2/h2c)")
	http10Flag          = flag.Bool("http1.0", false, "Use http1.0 (instead of http 1.1)")
	h2Flag              = flag.Bool("h2", false, "Attempt to use http2.0 / h2 (instead of http 1.1) for both TLS and h2c")
	httpsInsecureFlag   = flag.Bool("k", false, "Do not verify certs in https/tls/grpc connections")
	httpsInsecureFlagL  = flag.Bool("https-insecure", false, "Long form of the -k flag")
	resolve             = flag.String("resolve", "", "Resolve host name to this `IP`")
	headersFlags        headersFlagList
	httpOpts            fhttp.HTTPOptions
	followRedirectsFlag = flag.Bool("L", false, "Follow redirects (implies -std-client) - do not use for load test")
	userCredentialsFlag = flag.String("user", "", "User credentials for basic authentication (for http). Input data format"+
		" should be `user:password`")
	contentTypeFlag = flag.String("content-type", "",
		"Sets http content type. Setting this value switches the request method from GET to POST.")
	// PayloadSizeFlag is the value of -payload-size.
	PayloadSizeFlag = flag.Int("payload-size", 0, "Additional random payload size, replaces -payload when set > 0,"+
		" must be smaller than -maxpayloadsizekb. Setting this switches http to POST.")
	// PayloadFlag is the value of -payload.
	PayloadFlag = flag.String("payload", "", "Payload string to send along")
	// PayloadFileFlag is the value of -paylaod-file.
	PayloadFileFlag = flag.String("payload-file", "", "File `path` to be use as payload (POST for http), replaces -payload when set.")
	// PayloadStreamFlag for streaming payload from stdin (curl only).
	PayloadStreamFlag = flag.Bool("stream", false, "Stream payload from stdin (only for fortio curl mode)")
	// UnixDomainSocket to use instead of regular host:port.
	unixDomainSocketFlag = flag.String("unix-socket", "", "Unix domain socket `path` to use for physical connection")
	// CertFlag is the flag for the path for the client custom certificate.
	CertFlag = flag.String("cert", "", "`Path` to the certificate file to be used for client or server TLS")
	// KeyFlag is the flag for the path for the key for the `cert`.
	KeyFlag = flag.String("key", "", "`Path` to the key file matching the -cert")
	// CACertFlag is the flag for the path of the custom CA to verify server certificates in client calls.
	CACertFlag = flag.String("cacert", "",
		"`Path` to a custom CA certificate file to be used for the TLS client connections, "+
			"if empty, use https:// prefix for standard internet/system CAs")
	mTLS = flag.Bool("mtls", false, "Require client certificate signed by -cacert for client connections")
	// LogErrorsFlag determines if the non ok http error codes get logged as they occur or not.
	LogErrorsFlag = flag.Bool("log-errors", true, "Log http non 2xx/418 error codes as they occur")
	// RunIDFlag is optional RunID to be present in json results (and default json result filename if not 0).
	RunIDFlag = flag.Int64("runid", 0, "Optional RunID to add to json result and auto save filename, to match server mode")
	// HelpFlag is true if help/usage is being requested by the user.
	warmupFlag = flag.Bool("sequential-warmup", false,
		"http(s) runner warmup done in parallel instead of sequentially. When set, restores pre 1.21 behavior")
	curlHeadersStdout = flag.Bool("curl-stdout-headers", false,
		"Restore pre 1.22 behavior where http headers of the fast client are output to stdout in curl mode. now stderr by default.")
	// ConnectionReuseRange Dynamic string flag to set the max connection reuse range.
	ConnectionReuseRange = dflag.Flag("connection-reuse", dflag.New("",
		"Range `min:max` for the max number of connections to reuse for each thread, default to unlimited. "+
			"e.g. 10:30 means randomly choose a max connection reuse threshold between 10 and 30 requests.").
		WithValidator(ConnectionReuseRangeValidator(&httpOpts)))
	// NoReResolveFlag is false if we want to resolve the DNS name for each new connection.
	NoReResolveFlag = flag.Bool("no-reresolve", false, "Keep the initial DNS resolution and "+
		"don't re-resolve when making new connections (because of error or reuse limit reached)")
	MethodFlag = flag.String("X", "", "HTTP method to use instead of GET/POST depending on payload/content-type")
)

// SharedMain is the common part of main from fortio_main and fcurl.
// It sets up the common flags, the rest of usage/argument/flag handling
// is now moved to the [fortio.org/cli] and [fortio.org/scli] packages.
func SharedMain() {
	flag.Var(&headersFlags, "H",
		"Additional http header(s) or grpc metadata. Multiple `key:value` pairs can be passed using multiple -H.")
	flag.IntVar(&fhttp.BufferSizeKb, "httpbufferkb", fhttp.BufferSizeKb,
		"Size of the buffer (max data size) for the optimized http client in `kbytes`")
	flag.BoolVar(&fhttp.CheckConnectionClosedHeader, "httpccch", fhttp.CheckConnectionClosedHeader,
		"Check for Connection: Close Header")
	// FlagResolveIPType indicates which IP types to resolve.
	// With round robin resolution now the default, you are likely to get ipv6 which may not work if
	// use both type (`ip`). In particular some test environments like the CI do have ipv6
	// for localhost but fail to connect. So we made the default ip4 only.
	dflag.Flag("resolve-ip-type", fnet.FlagResolveIPType)
	// FlagResolveMethod decides which method to use when multiple ips are returned for a given name
	// default assumes one gets all the ips in the first call and does round robin across these.
	// first just picks the first answer, rr rounds robin on each answer.
	dflag.Flag("dns-method", fnet.FlagResolveMethod)
	dflag.Flag("echo-server-default-params", fhttp.DefaultEchoServerParams)
	dflag.FlagBool("proxy-all-headers", fhttp.Fetch2CopiesAllHeader)
	dflag.Flag("server-idle-timeout", fhttp.ServerIdleTimeout)
	// MaxDelay is the maximum delay allowed for the echoserver responses.
	// It is a dynamic flag with default value of 1.5s so we can test the default 1s timeout in envoy.
	dflag.Flag("max-echo-delay", fhttp.MaxDelay)
	// call [scli.ServerMain()] to complete the setup.
}

// FetchURL is fetching url content and exiting with 1 upon error.
// common part between fortio_main and fcurl.
func FetchURL(o *fhttp.HTTPOptions) {
	// keepAlive could be just false when making 1 fetch but it helps debugging
	// the http client when making a single request if using the flags
	o.DataWriter = os.Stdout
	client, _ := fhttp.NewClient(o)
	// big gotcha that nil client isn't nil interface value (!)
	if client == nil || reflect.ValueOf(client).IsNil() {
		os.Exit(1) // error logged already
	}
	var code int
	var dataLen int64
	var header uint
	if client.HasBuffer() {
		// Fast client
		var data []byte
		var headerI int
		code, data, headerI = client.Fetch(context.Background())
		dataLen = int64(len(data))
		header = uint(headerI)
		if *curlHeadersStdout {
			os.Stdout.Write(data)
		} else {
			os.Stderr.Write(data[:header])
			os.Stdout.Write(data[header:])
		}
	} else {
		code, dataLen, header = client.StreamFetch(context.Background())
	}
	log.LogVf("Fetch result code %d, data len %d, headerlen %d", code, dataLen, header)
	if code != http.StatusOK {
		log.Errf("Error status %d", code)
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

// ConnectionReuseRangeValidator returns a validator function that checks if the connection reuse range is valid
// and set in httpOpts.
func ConnectionReuseRangeValidator(httpOpts *fhttp.HTTPOptions) func(string) error {
	return func(value string) error {
		return httpOpts.ValidateAndSetConnectionReuseRange(value)
	}
}

// SharedHTTPOptions is the flag->httpoptions transfer code shared between
// fortio_main and fcurl.
func SharedHTTPOptions() *fhttp.HTTPOptions {
	url := strings.TrimLeft(flag.Arg(0), " \t\r\n")
	httpOpts.URL = url
	httpOpts.HTTP10 = *http10Flag
	httpOpts.H2 = *h2Flag
	httpOpts.DisableFastClient = *stdClientFlag
	httpOpts.DisableKeepAlive = !*keepAliveFlag
	httpOpts.AllowHalfClose = *halfCloseFlag
	httpOpts.Compression = *compressionFlag
	httpOpts.HTTPReqTimeOut = *httpReqTimeoutFlag
	httpOpts.Insecure = TLSInsecure()
	httpOpts.Resolve = *resolve
	httpOpts.UserCredentials = *userCredentialsFlag
	if len(*contentTypeFlag) > 0 {
		// only set content-type from flag if flag isn't empty as it can come also from -H content-type:...
		httpOpts.ContentType = *contentTypeFlag
	}
	if *PayloadStreamFlag {
		httpOpts.PayloadReader = os.Stdin
	} else {
		// Returns nil if file read error, an empty but non nil slice if no payload is requested.
		httpOpts.Payload = fnet.GeneratePayload(*PayloadFileFlag, *PayloadSizeFlag, *PayloadFlag)
		if httpOpts.Payload == nil {
			// Error already logged
			os.Exit(1)
		}
	}
	httpOpts.UnixDomainSocket = *unixDomainSocketFlag
	if *followRedirectsFlag {
		httpOpts.FollowRedirects = true
		httpOpts.DisableFastClient = true
	}
	httpOpts.CACert = *CACertFlag
	httpOpts.Cert = *CertFlag
	httpOpts.Key = *KeyFlag
	httpOpts.MTLS = *mTLS
	httpOpts.LogErrors = *LogErrorsFlag
	httpOpts.SequentialWarmup = *warmupFlag
	httpOpts.NoResolveEachConn = *NoReResolveFlag
	httpOpts.MethodOverride = *MethodFlag
	return &httpOpts
}
