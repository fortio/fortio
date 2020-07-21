// Copyright 2016 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package main

import (
	"fmt"
	"html/template"
	"net/http"

	"flag"

	"fortio.org/fortio/dflag"
	"fortio.org/fortio/dflag/configmap"
	"fortio.org/fortio/dflag/endpoint"
	"fortio.org/fortio/log"
)

var (
	listenPort = flag.Int("port", 8080, "Port the example server listens on.")
	listenHost = flag.String("host", "0.0.0.0", "Host to bind the example server to.")

	dirPathWatch = flag.String("dflag_dir_path", "/tmp/foobar", "path to dir to watch updates from.")

	staticInt = flag.Int("example_my_static_int", 1337, "Something integery here.")

	dynStr = dflag.DynString(flag.CommandLine, "example_my_dynamic_string", "initial_value", "Something interesting here.")
	dynInt = dflag.DynInt64(flag.CommandLine, "example_my_dynamic_int", 1337, "Something integery here.")

	// This is an example of a dynamically-modifiable JSON flag of an arbitrary type.
	dynJSON = dflag.DynJSON(
		flag.CommandLine,
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

	flag.Parse()
	u, err := configmap.Setup(flag.CommandLine, *dirPathWatch)
	if err != nil {
		log.Fatalf("Failed setting up an updater %v", err)
	}
	defer u.Stop()
	setUrl := "/debug/flags/set"
	dflagEndpoint := endpoint.NewFlagsEndpoint(flag.CommandLine, setUrl)
	http.HandleFunc("/debug/flags", dflagEndpoint.ListFlags)
	http.HandleFunc(setUrl, dflagEndpoint.SetFlag)
	http.HandleFunc("/", handleDefaultPage)

	addr := fmt.Sprintf("%s:%d", *listenHost, *listenPort)
	log.Infof("Serving at: %v", addr)
	if err := http.ListenAndServe(addr, http.DefaultServeMux); err != nil {
		log.Fatalf("Failed serving: %v", err)
	}
	log.Infof("Done, bye.")
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
