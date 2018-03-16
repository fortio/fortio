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

package fnet

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
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
)

// TLSInfo prepares tls.Config's from TLS filename inputs.
type TLSInfo struct {
	CAFiles  []string
	CertFile string
	KeyFile  string
}

// ClientConfig returns a tls.Config for client use.
func (info *TLSInfo) ClientConfig() (*tls.Config, error) {
	// CA for verifying the server
	pool, err := NewCertPool(info.CAFiles)
	if err != nil {
		return nil, err
	}
	// client certificate (for authentication)
	cert, err := tls.LoadX509KeyPair(info.CertFile, info.KeyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
		RootCAs:            pool,                    // CA bundle the client should trust when verifying a server
		Certificates:       []tls.Certificate{cert}, // Client certificates to authenticate to the server
	}, nil
}

// ServerConfig returns a tls.Config for server use.
func (info *TLSInfo) ServerConfig() (*tls.Config, error) {
	// server certificate to present to clients
	cert, err := tls.LoadX509KeyPair(info.CertFile, info.KeyFile)
	if err != nil {
		return nil, err
	}
	// CA for authenticating clients
	pool, err := NewCertPool(info.CAFiles)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},        // Certificates the server should present to clients
		ClientAuth:   tls.RequireAndVerifyClientCert, // Client Authentication (required)
		ClientCAs:    pool,                           // CA for verifying and authorizing client certificates
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	}, nil
}

// NewCertPool creates an x509 certPool with the provided CA files.
func NewCertPool(CAFiles []string) (*x509.CertPool, error) {
	certPool := x509.NewCertPool()

	for _, CAFile := range CAFiles {
		pemByte, err := ioutil.ReadFile(CAFile)
		if err != nil {
			return nil, err
		}

		for {
			var block *pem.Block
			block, pemByte = pem.Decode(pemByte)
			if block == nil {
				break
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			certPool.AddCert(cert)
		}
	}

	return certPool, nil
}

// NewCredentials creates new client/server TlS credentials based on provided type and files.
func NewCredentials(client bool, ca []string, cert, key string) (tls *tls.Config, err error) {
	tlsInfo := TLSInfo{
		CAFiles:  ca,
		CertFile: cert,
		KeyFile:  key,
	}
	if client {
		return tlsInfo.ClientConfig()
	}
	return tlsInfo.ServerConfig()
}
