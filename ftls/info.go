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

package ftls

import (
	"crypto/tls"

	"istio.io/fortio/log"
)

const (
	// DefaultServerCert is the default full path of the server-side certificate.
	DefaultServerCert = "/etc/ssl/certs/server.crt"
	// DefaultServerKey is the default full path of the server-side key.
	DefaultServerKey = "/etc/ssl/certs/server.key"
	// DefaultClientCert is the default full path of the client-side certificate.
	DefaultClientCert = "/etc/ssl/certs/client.crt"
	// DefaultClientKey is the default full path of the client-side key.
	DefaultClientKey = "/etc/ssl/certs/client.key"
	// DefaultCACert is the default full path of the Certificate Authority certificate.
	DefaultCACert = "/etc/ssl/certs/ca.crt"
)

// TLSInfo prepares tls.Config's from TLS filename inputs.
type TLSInfo struct {
	CAFile   string
	CertFile string
	KeyFile  string
}

// ClientConfig returns a tls.Config for client use.
func (info *TLSInfo) ClientConfig() (*tls.Config, error) {
	// CA for verifying the server
	pool, err := NewCertPool([]string{info.CAFile})
	if err != nil {
		return nil, err
	}
	log.Infof("Using TLS client certificate: %v", DefaultClientCert)
	log.Infof("Using TLS client key: %v", DefaultClientKey)
	log.Infof("Using CA certificate: %v to authenticate server certificate", DefaultCACert)
	// client certificate (for authentication)
	cert, err := tls.LoadX509KeyPair(info.CertFile, info.KeyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		// CA bundle the client should trust when verifying a server
		RootCAs: pool,
		// Client certificates to authenticate to the server
		Certificates: []tls.Certificate{cert},
	}, nil
}

// ServerConfig returns a tls.Config for server use.
func (info *TLSInfo) ServerConfig() (*tls.Config, error) {
	// server certificate to present to clients
	cert, err := tls.LoadX509KeyPair(info.CertFile, info.KeyFile)
	if err != nil {
		return nil, err
	}
	log.Infof("Using TLS server certificate: %v", DefaultServerCert)
	log.Infof("Using TLS server key: %v", DefaultServerKey)
	log.Infof("Using CA certificate: %v to authenticate server certificate", DefaultCACert)
	// CA for authenticating clients
	pool, err := NewCertPool([]string{info.CAFile})
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Certificates the server should present to clients
		Certificates: []tls.Certificate{cert},
		// Client Authentication (required)
		ClientAuth: tls.RequireAndVerifyClientCert,
		// CA for verifying and authorizing client certificates
		ClientCAs: pool,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}, nil
}
