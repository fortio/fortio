// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"flag"

	"fortio.org/fortio/dflag"
	"fortio.org/fortio/dflag/configmap"
)

var (
	serverFlags = flag.NewFlagSet("server_flags", flag.ContinueOnError)

	listenPort = serverFlags.Int("port", 8080, "Port the example server listens on.")
	listenHost = serverFlags.String("host", "0.0.0.0", "Host to bind the example server to.")

	dirPathWatch = serverFlags.String("dflag_dir_path", "/tmp/foobar", "path to dir to watch updates from.")

	staticInt = serverFlags.Int("example_my_static_int", 1337, "Something integery here.")

	dynStr = dflag.DynString(serverFlags, "example_my_dynamic_string", "initial_value", "Something interesting here.")
	dynInt = dflag.DynInt64(serverFlags, "example_my_dynamic_int", 1337, "Something integery here.")

	// This is an example of a dynamically-modifiable JSON flag of an arbitrary type.
	dynJSON = dflag.DynJSON(
		serverFlags,
		"example_my_dynamic_json",
		&exampleConfig{
			Policy: "allow",
			Rate:   50,
			Entries: []*exampleEntry{
				{Name: "foobar", Allowed: true},
			}},
		"An arbitrary JSON struct.")
)

func main() {
	logger := log.New(os.Stderr, "server", log.LstdFlags)
	if err := serverFlags.Parse(os.Args[1:]); err != nil {
		logger.Fatalf("%v", err)
	}

	u, err := configmap.New(serverFlags, *dirPathWatch, logger)
	if err != nil {
		logger.Fatalf("Failed setting up an updater %v", err)
	}
	err = u.Initialize()
	if err != nil {
		logger.Fatalf("Failed initializing watcher %v", err)
	}
	if err := u.Start(); err != nil {
		logger.Fatalf("Could not start: %v", err)
	}
	logger.Printf("configmap flag value watching initialized")

	dflagEndpoint := dflag.NewStatusEndpoint(serverFlags)
	http.HandleFunc("/debug/dflag", dflagEndpoint.ListFlags)
	http.HandleFunc("/", handleDefaultPage)

	addr := fmt.Sprintf("%s:%d", *listenHost, *listenPort)
	logger.Printf("Serving at: %v", addr)
	if err := http.ListenAndServe(addr, http.DefaultServeMux); err != nil {
		logger.Fatalf("Failed serving: %v", err)
	}
	logger.Printf("Done, bye.")
}

var (
	defaultPage = template.Must(template.New("default_page").Parse(
		`
<html><head>
<title>Example Server</title>
<link href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.4/css/bootstrap.css" rel="stylesheet">

</head>
<body>
</body>
	<h1>String value {{ .DynString }}</h1>
	<h1>I'm such a number {{ .DynInt }}</h1>
	<h1>Default policy: {{ .Policy }} and number of entries {{ .NumEntries }}.
</html>
`))
)

func handleDefaultPage(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
	resp.Header().Add("Content-Type", "text/html")

	actualJSON, _ := dynJSON.Get().(*exampleConfig)
	err := defaultPage.Execute(resp, map[string]interface{}{
		"DynString":  dynStr.Get(),
		"DynInt":     dynInt.Get(),
		"Policy":     actualJSON.Policy,
		"NumEntries": len(actualJSON.Entries),
	})
	if err != nil {
		panic(err)
	}
}

type exampleConfig struct {
	Policy  string          `json:"policy"`
	Rate    int             `json:"rate"`
	Entries []*exampleEntry `json:"entries"`
}

type exampleEntry struct {
	Name    string `json:"entry"`
	Allowed bool   `json:"bool"`
}
