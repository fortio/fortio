// Copyright 2022 Fortio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rapi // import "fortio.org/fortio/rapi"

import (
	"bytes"
	"crypto/md5" // nolint: gosec // md5 is mandated by tsv format, not our choice
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/fortio/log"
)

// DataList returns the .json files/entries in data dir.
func DataList() (dataList []string) {
	files, err := ioutil.ReadDir(dataDir)
	if err != nil {
		log.Critf("Can list directory %s: %v", dataDir, err)
		return
	}
	// Newest files at the top:
	for i := len(files) - 1; i >= 0; i-- {
		name := files[i].Name()
		ext := ".json"
		if !strings.HasSuffix(name, ext) || files[i].IsDir() {
			log.LogVf("Skipping non %s file: %s", ext, name)
			continue
		}
		dataList = append(dataList, name[:len(name)-len(ext)])
	}
	log.LogVf("data list is %v (out of %d files in %s)", dataList, len(files), dataDir)
	return dataList
}

type tsvCache struct {
	cachedDirTime time.Time
	cachedResult  []byte
}

var (
	gTSVCache      tsvCache
	gTSVCacheMutex = &sync.Mutex{}
	// Starts and end with / where the UI is running from, prefix to data etc.
	uiPath string
	// Base URL used for index - useful when running under an ingress with prefix. can be empty otherwise.
	baseURL string
)

// format for gcloud transfer
// https://cloud.google.com/storage/transfer/create-url-list
func SendTSVDataIndex(urlPrefix string, w http.ResponseWriter) {
	info, err := os.Stat(dataDir)
	if err != nil {
		log.Errf("Unable to stat %s: %v", dataDir, err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	gTSVCacheMutex.Lock() // Kind of a long time to hold a lock... hopefully the FS doesn't hang...
	useCache := (info.ModTime() == gTSVCache.cachedDirTime) && (len(gTSVCache.cachedResult) > 0)
	if !useCache {
		var b bytes.Buffer
		b.Write([]byte("TsvHttpData-1.0\n"))
		for _, e := range DataList() {
			fname := e + ".json"
			f, err := os.Open(path.Join(dataDir, fname))
			if err != nil {
				log.Errf("Open error for %s: %v", fname, err)
				continue
			}
			// nolint: gosec // This isn't a crypto hash, more like a checksum - and mandated by the spec above, not our choice
			h := md5.New()
			var sz int64
			if sz, err = io.Copy(h, f); err != nil {
				f.Close()
				log.Errf("Copy/read error for %s: %v", fname, err)
				continue
			}
			b.Write([]byte(urlPrefix))
			b.Write([]byte(fname))
			b.Write([]byte("\t"))
			b.Write([]byte(strconv.FormatInt(sz, 10)))
			b.Write([]byte("\t"))
			b.Write([]byte(base64.StdEncoding.EncodeToString(h.Sum(nil))))
			b.Write([]byte("\n"))
		}
		gTSVCache.cachedDirTime = info.ModTime()
		gTSVCache.cachedResult = b.Bytes()
	}
	result := gTSVCache.cachedResult
	lastModified := gTSVCache.cachedDirTime.Format(http.TimeFormat)
	gTSVCacheMutex.Unlock()
	log.Infof("Used cached %v to serve %d bytes TSV", useCache, len(result))
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	// Cloud transfer requires ETag
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", lastModified))
	w.Header().Set("Last-Modified", lastModified)
	_, _ = w.Write(result)
}

func sendHTMLDataIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	_, _ = w.Write([]byte("<html><body><ul>\n"))
	for _, e := range DataList() {
		_, _ = w.Write([]byte("<li><a href=\""))
		_, _ = w.Write([]byte(e))
		_, _ = w.Write([]byte(".json\">"))
		_, _ = w.Write([]byte(e))
		_, _ = w.Write([]byte("</a>\n"))
	}
	_, _ = w.Write([]byte("</ul></body></html>"))
}

// GetDataURL gives the url of the data/ dir either using configured `-base-url` and ui path
// from from the incoming Host header.
func GetDataURL(r *http.Request) string {
	// Ingress effect / baseURL support:
	url := baseURL
	if len(url) == 0 {
		// The Host header includes original host/port, only missing is the proto:
		proto := r.Header.Get("X-Forwarded-Proto")
		if len(proto) == 0 {
			proto = "http"
		}
		url = proto + "://" + r.Host
	}
	return url + uiPath + "data/" // base has been cleaned of trailing / in fortio_main
}

func ID2URL(r *http.Request, id string) string {
	if id == "" {
		return ""
	}
	return GetDataURL(r) + id + ".json"
}

// LogAndFilterDataRequest logs the data request.
func LogAndFilterDataRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fhttp.LogRequest(r, "Data")
		path := r.URL.Path
		if strings.HasSuffix(path, "/") || strings.HasSuffix(path, "/index.html") {
			sendHTMLDataIndex(w)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		ext := "/index.tsv"
		if strings.HasSuffix(path, ext) {
			urlPrefix := GetDataURL(r)
			log.Infof("Prefix is '%s'", urlPrefix)
			SendTSVDataIndex(urlPrefix, w)
			return
		}
		if !strings.HasSuffix(path, ".json") {
			log.Warnf("Filtering request for non .json '%s'", path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fhttp.CacheOn(w)
		h.ServeHTTP(w, r)
	})
}

func AddDataHandler(mux *http.ServeMux, baseurl, uipath, datadir string) {
	gTSVCacheMutex.Lock()
	gTSVCache.cachedResult = []byte{}
	baseURL = baseurl
	uiPath = uipath
	SetDataDir(datadir)
	gTSVCacheMutex.Unlock()
	if datadir == "" {
		log.Infof("No data dir so no handler for data")
	}
	fs := http.FileServer(http.Dir(datadir))
	mux.Handle(uiPath+"data/", LogAndFilterDataRequest(http.StripPrefix(uiPath+"data", fs)))
	if datadir == "." {
		var err error
		datadir, err = os.Getwd()
		if err != nil {
			log.Errf("Unable to get current directory: %v", err)
		}
	}
	log.Printf("Data directory is %s", datadir)
}
