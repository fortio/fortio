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
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
)

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
func NewCredentials(client bool, cert, key, ca string) (tls *tls.Config, err error) {
	tlsInfo := TLSInfo{
		CertFile: cert,
		KeyFile:  key,
		CAFile:   ca,
	}
	if client {
		return tlsInfo.ClientConfig()
	}
	return tlsInfo.ServerConfig()
}
