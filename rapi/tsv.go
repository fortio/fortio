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

	"fortio.org/fortio/log"
)

// TODO: The breakdown of what is in this package vs rest of the code
// in original ui/uihandler.go is not clean/move isn't complete

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
