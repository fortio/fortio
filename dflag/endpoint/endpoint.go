// Copyright 2015 Michal Witkowski. All Rights Reserved.
// See LICENSE for licensing terms.

package endpoint

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"fortio.org/fortio/dflag"
	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/log"
)

// FlagsEndpoint is a collection of `http.HandlerFunc` that serve debug pages about a given `FlagSet.
type FlagsEndpoint struct {
	flagSet *flag.FlagSet
	setURL  string
}

// NewFlagsEndpoint creates a new debug `http.HandlerFunc` collection for a given `FlagSet`
// and an optional URL for Setter (needs to be secured). if setURL is empty, no setter function
// will be enabled.
func NewFlagsEndpoint(flagSet *flag.FlagSet, setURL string) *FlagsEndpoint {
	return &FlagsEndpoint{flagSet: flagSet, setURL: setURL}
}

func HttpErrf(resp http.ResponseWriter, statusCode int, message string, rest ...interface{}) {
	resp.WriteHeader(statusCode)
	resp.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	log.Errf(message, rest...)
	resp.Write([]byte(fmt.Sprintf(message, rest...)))
}

// SetFlag updates a dynamic flag to a new value.
func (e *FlagsEndpoint) SetFlag(resp http.ResponseWriter, req *http.Request) {
	fhttp.LogRequest(req, "SetFlag")
	if e.setURL == "" {
		HttpErrf(resp, http.StatusForbidden, "setting flags is not enabled")
		return
	}
	name := req.URL.Query().Get("name")
	value := req.URL.Query().Get("value")
	f := e.flagSet.Lookup(name)
	if f == nil {
		HttpErrf(resp, http.StatusForbidden, "Flag %q not found", name)
		return
	}
	if !dflag.IsFlagDynamic(f) {
		HttpErrf(resp, http.StatusBadRequest, "Trying to set non dynamic flag %q", name)
		return
	}
	if err := e.flagSet.Set(name, value); err != nil {
		HttpErrf(resp, http.StatusNotAcceptable, "Error setting %q to %q: %v", name, value, err)
		return
	}
	resp.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	resp.Write([]byte(fmt.Sprintf("Success %q -> %q", name, value)))
}

// ListFlags provides an HTML and JSON `http.HandlerFunc` that lists all Flags of a `FlagSet`.
// Additional URL query parameters can be used such as `type=[dynamic,static]` or `only_changed=true`.
func (e *FlagsEndpoint) ListFlags(resp http.ResponseWriter, req *http.Request) {
	fhttp.LogRequest(req, "ListFlags")

	onlyChanged := req.URL.Query().Get("only_changed") != ""
	onlyDynamic := req.URL.Query().Get("type") == "dynamic"
	onlyStatic := req.URL.Query().Get("type") == "static"

	flagSetJSON := &flagSetJSON{}
	e.flagSet.VisitAll(func(f *flag.Flag) {
		if onlyChanged && f.Value.String() == f.DefValue { // not exactly the same as "changed" (!)
			return
		}
		if onlyDynamic && !dflag.IsFlagDynamic(f) {
			return
		}
		if onlyStatic && dflag.IsFlagDynamic(f) {
			return
		}
		flagSetJSON.Flags = append(flagSetJSON.Flags, flagToJSON(f))
	})
	flagSetJSON.ChecksumDynamic = fmt.Sprintf("%x", dflag.ChecksumFlagSet(e.flagSet, dflag.IsFlagDynamic))
	flagSetJSON.ChecksumStatic = fmt.Sprintf("%x", dflag.ChecksumFlagSet(e.flagSet, func(f *flag.Flag) bool { return !dflag.IsFlagDynamic(f) }))
	flagSetJSON.FlagSetURL = e.setURL

	if requestIsBrowser(req) && req.URL.Query().Get("format") != "json" {
		resp.WriteHeader(http.StatusOK)
		resp.Header().Add("Content-Type", "text/html")
		if err := dflagListTemplate.Execute(resp, flagSetJSON); err != nil {
			log.Fatalf("Bad template evaluation: %v", err)
		}
	} else {
		resp.Header().Add("Content-Type", "application/json")
		out, err := json.MarshalIndent(&flagSetJSON, "", "  ")
		if err != nil {
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp.WriteHeader(http.StatusOK)
		resp.Write(out)
	}
}

func requestIsBrowser(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept"), "html")
}

var (
	dflagListTemplate = template.Must(template.New("dflag_list").Parse(
		`
<html><head>
<title>Flags List</title>
<link href="https://maxcdn.bootstrapcdn.com/bootstrap/3.3.4/css/bootstrap.css" rel="stylesheet">

</head>
<body>
<div class="container-fluid">
<div class="col-md-10 col-md-offset-1">
	<h1>Flags Debug View</h1>
	<p>
	This page presents the configuration flags of this server (<a href="?format=json">JSON</a>).
	</p>
	<p>
	You can easily filter only <a href="?only_changed=true"><span class="label label-primary">changed</span> flag</a> or filter flags by type:
	</p>
	<ul>
	  <li><a href="?type=dynamic"><span class="label label-success">dynamic</span></a> - flags tweakable dynamically - checksum <code>{{ .ChecksumDynamic }}</code></li>
	  <li><a href="?type=static"><span class="label label-default">static</span></a> - initialization-time only flags - checksum <code>{{ .ChecksumStatic }}</code></li>
	</ul>



	{{range $flag := .Flags }}
		<div class="panel panel-default">
          <div class="panel-heading">
            <code>{{ $flag.Name }}</code>
            {{ if $flag.IsChanged }}<span class="label label-primary">changed</span>{{ end }}
            {{ if $flag.IsDynamic }}
                <span class="label label-success">dynamic</span>
            {{ else }}
                <span class="label label-default">static</span>
            {{ end }}

          </div>
		  <div class="panel-body">
		    <dl class="dl-horizontal" style="margin-bottom: 0px">
			  <dt>Description</dt>
			  <dd><small>{{ $flag.Description }}</small></dd>
			  <dt>Default</dt>
			  <dd><pre style="font-size: 8pt">{{ $flag.DefaultValue }}</pre></dd>
			  <dt>Current</dt>
			  {{ if and $flag.IsDynamic (ne $.FlagSetURL "") }}
			  <form action="{{ $.FlagSetURL }}">
			  <input type="hidden" name="name" value="{{ $flag.Name }}" />
			  <dd><pre class="success" style="font-size: 8pt"><input type="text" name="value" value="{{ $flag.CurrentValue }}" /></pre></dd>
			  </form>
			  {{ else }}
			  <dd><pre class="success" style="font-size: 8pt">{{ $flag.CurrentValue }}</pre></dd>
			  {{ end }}
		    </dl>
		  </div>
		</div>
	{{end}}
</div></div>
</body>
</html>
`))
)

type flagSetJSON struct {
	ChecksumStatic  string      `json:"checksum_static"`
	ChecksumDynamic string      `json:"checksum_dynamic"`
	FlagSetURL      string      `json:"set_url"`
	Flags           []*flagJSON `json:"flags"`
}

type flagJSON struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	CurrentValue string `json:"current_value"`
	DefaultValue string `json:"default_value"`

	IsChanged bool `json:"is_changed"`
	IsDynamic bool `json:"is_dynamic"`
}

func flagToJSON(f *flag.Flag) *flagJSON {
	fj := &flagJSON{
		Name:         f.Name,
		Description:  f.Usage,
		CurrentValue: f.Value.String(),
		DefaultValue: f.DefValue,
		IsChanged:    f.Value.String() != f.DefValue,
		IsDynamic:    dflag.IsFlagDynamic(f),
	}
	if dj, ok := f.Value.(dflag.DynamicJSONFlagValue); ok {
		_ = dj.IsJSON() // could assert true
		fj.CurrentValue = prettyPrintJSON(fj.CurrentValue)
		fj.DefaultValue = prettyPrintJSON(fj.DefaultValue)
	}
	return fj
}

func prettyPrintJSON(input string) string {
	out := &bytes.Buffer{}
	if err := json.Indent(out, []byte(input), "", "  "); err != nil {
		return "PRETTY_ERROR"
	}
	return out.String()
}
