// Copyright 2017 Istio Authors.
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
//

// Adapted from istio/proxy/test/backend/echo with error handling and
// concurrency fixes and making it as low overhead as possible
// (no std output by default)

package storage // import "istio.io/fortio/storage"

import (
	"time"

	"istio.io/fortio/log"

	"cloud.google.com/go/datastore"
	"golang.org/x/net/context"
)

// DatastoreClient is a struct for interacting with Google Cloud Datastore
type DatastoreClient struct {
	client *datastore.Client
}

type IstioPerfResult struct {
	TestType       string    `datastore:"test_type"`
	TestTime       time.Time `datastore:"test_time"`
	QPS            string    `datastore:"qps"`
	Duration       string    `datastore:"duration"`
	HasMTLS        bool      `datastore:"has_mtls"`
	NumConnections int       `datastore:"num_connections"`
	URL            string    `datastore:"url"`
	Label          string    `datastore:"label"`
	ResultJSON     string    `datastore:"result_json,noindex"`
}

// NewDatastoreClient creates a new DatastoreClient instance
func NewDatastoreClient(ctx context.Context, projectID string) (*DatastoreClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	// Creates a Datastore client.
	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		log.Errf("Failed to create client: %v", err)
		return nil, err
	}

	c := &DatastoreClient{
		client: client,
	}
	return c, nil
}

// UploadResult uploads a test result to Cloud Datastore
func (c *DatastoreClient) UploadResult(ctx context.Context, result *IstioPerfResult) error {
	if result.TestTime.IsZero() {
		result.TestTime = time.Now()
	}

	_, err := c.client.Put(ctx, datastore.IncompleteKey("IstioPerf", nil), result)
	return err
}
