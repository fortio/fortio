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

package main

import (
	"io/ioutil"
	"os"

	"istio.io/fortio/log"
)

const (
	caCrt = `
-----BEGIN CERTIFICATE-----
MIIFDTCCAvWgAwIBAgIJAOwclmyo8AugMA0GCSqGSIb3DQEBCwUAMBIxEDAOBgNV
BAMMB2Zha2UtY2EwHhcNMTgwMzMxMDA0NDA5WhcNMjgwMzI4MDA0NDA5WjASMRAw
DgYDVQQDDAdmYWtlLWNhMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA
raRSrt3pCbpT80Ddip1NDl4O59gfVSesy4RJJw/tROnDbxJdPVfdj2lwizTQq1ng
7a5RkzERg5cUAjNDjo3k3iyYdWr5ugIQrbdN9vNBPoSu+HB6LSGr4C+veiZEWm5j
NnH88OG03PiWUce1MoT83APQyG6D7IRhCz2pWbMSLfXZMgHAyRR0FERqqGTgaOIk
g8e2aC3bLmgylKncfose6Ex+uk3wOPfy/co4nDg9qG4+ZjxxH5Qw7iXA/rn4a1vE
3qnIW1HmuEGqJTenkiREb7AWvm/ZUwtD0iuKNk592mJ6ZXYHFhBQMhOxwWDEZwVp
QfZaa/kIw3JItLwN5amBLLnCKP32P4Uf/8ZadNwvcdyq8ggnGKfODVJCu432l8b7
Lf3CEY8dMyukoZBjDEbXNRgEhNVVKP0nEC6eEnR0Hth+vgI3OFTuAs8GIjH0zNag
qGOe89vzJ6KkTvlw64TBbJynjbjkxIFtjmvOXpbP06lUjBhMQNvXR/QfJICah9FH
TAfYBB2Rvh3uStvnkOFiExgI+G/vPPwH0hGByew8m6Wz7NtDr2G3igZS2o/+frtE
Bpy9yibFHIxZqAQthuBHKI9r/LpglyQZS1JukMlTBbtaxVgUXld8A9SJXl2oQJY1
/915KS/SiwApCWig6cx1KUcYdSB04lqG5nt7mArqBlECAwEAAaNmMGQwHQYDVR0O
BBYEFCgckdS052xtzIPm8wgy/gXlEVfAMB8GA1UdIwQYMBaAFCgckdS052xtzIPm
8wgy/gXlEVfAMBIGA1UdEwEB/wQIMAYBAf8CAQAwDgYDVR0PAQH/BAQDAgGGMA0G
CSqGSIb3DQEBCwUAA4ICAQBx+75xOmuosP7vlxuO0i5ejdRZG24JXi9a9HDTzkAS
R0vhb/Vk6I5OiZfE2KUgAlbfX45StnP2GYOgcHNr5lH2kLlzPsvczUDtzwO3iK4Y
EKG2YHNVOgzcv/oylE+9B/pYd41oFfvJAcIwOOBzuzj99VqjRwp/J5sEVAvrEsl1
OdeHX8w7M4xhoSP3dsKvQV6WOnYJSL4oinE1R26q9mJYFHwqLUR3k7yU1T39iKeQ
3F+Rq0D3C8zyzC/F8+9TGc7A/qCemz7BBwL/5rRJQ2vW/HxLlD54l8tcf+BtCFu/
lTs7T/b7XOcJACZ4NjSV4n12+YqLkjUQTdmwlZ9Z7VGG/hDnw678lFlAjuALogHc
ds/eIdM3tO1vXvT73mNyhtcYOPSDI/4ZTYuqzW010Fk6358H3fw0C4MLLTpJPSKv
suCyBL7MOX70axVvYF1Cr47hCa2HaUtOrHw8I3F3nAXw0JWFFu9/96hAbJr7Z9II
LiaCRsTZqIO+yWe5wJnlFdmi51U0cqjo9jVdU1LCpG16dYxYxZ6j9rA+8y0LhDu+
GuvJkWzGsry7V/oG/u/7l4jhXHa0olxFlaF900hbFMsSiLUEHngFyFZrIFIhCRJ5
RwkKQPiq6nlbIB+Hb57BQbBTsl5YYzfK6SGpTZ2dQ0+FFvF7KJA473kmhmCB9bYt
Hw==
-----END CERTIFICATE-----`

	svrCrt = `
-----BEGIN CERTIFICATE-----
MIIEnzCCAoegAwIBAgICEAAwDQYJKoZIhvcNAQELBQAwEjEQMA4GA1UEAwwHZmFr
ZS1jYTAeFw0xODAzMzEwMDQ0MDlaFw0xOTAzMzEwMDQ0MDlaMBYxFDASBgNVBAMM
C2Zha2Utc2VydmVyMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA483d
AGr4vGI2SNjVdebGtyV4b7h1cA6zrhzvtrgGg4/OYHyD04BA43flZ26rIjdkXyoS
NWe5e+RNyqJ/K/afL5JfWzei8EpZkbtlhZenDPA/MSQKV1GSeWjPRkwsEqMbdJeI
cQwmhdIaxcXaNDXS40WcRXZydcgXD9X54WZ2c7Nr6fG3RJ6zVi0tBipCLa5mCz5K
VKMkktwD9yq7C4AfchabG24xBcR9rcD4issUPL+TizljmbxSwKj1bqo/4A920V6F
zUWjkDMJSA1lhfLsU+nqlc8cK349dxPWCAdSvKxoP4A1gMO5oz15Tp6OZQIbV1jw
MkkJjFoZ8YGiYtpAawIDAQABo4H6MIH3MAkGA1UdEwQCMAAwEQYJYIZIAYb4QgEB
BAQDAgZAMDMGCWCGSAGG+EIBDQQmFiRPcGVuU1NMIEdlbmVyYXRlZCBTZXJ2ZXIg
Q2VydGlmaWNhdGUwHQYDVR0OBBYEFAmJG2Q69qMbAlT38QC3N4HJJ7mvMEIGA1Ud
IwQ7MDmAFCgckdS052xtzIPm8wgy/gXlEVfAoRakFDASMRAwDgYDVQQDDAdmYWtl
LWNhggkA7ByWbKjwC6AwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsGAQUF
BwMBMBoGA1UdEQQTMBGCCWxvY2FsaG9zdIcEfwAAATANBgkqhkiG9w0BAQsFAAOC
AgEAj4pUhua9eUmft5i4YHSYGYDQd48WYz1hmWgYpWWMLtD4mxYSZpzEfYc3S0wi
rEqexBqBKUAdkP9908iPiiok0GTnqHyVvGr0XHkMTUeqXFq83EqcmSyf+OZtrZAp
ha31t5R4t+HlYYHUo/RohOrNF3wIB3Vi38H6LR6RMl9tdXDl2b+qLl+zz/XOkYJ6
DJwV8ThgHWC99cQbdtaHysrPgbTSQ03QaHSk4rwyqtiXAMvENPk2k445R7oMe5EV
Is6YKK1GoChVZjU1sBVLk8SIoo2y41obhU0q0Y5EyYpRG/V08zEZBEpPdt2IldYD
a5F+DmURjOnR+NtzdKP5phJ+K8UgSNXGCRT0BgPO6Q1nPtQXiEE7oeHq0p5TCrdo
Uj45aFnC7ZIRahr4jwccZelCP2IXy7IhEedX5eb70Ju7KCb68qmcY5xTZ+fDmI6f
zTmgb1H/VeQQVjX5DcrKi6JETVBppmCoNtOILalPkbF6IS6kXvCgVRk9GCTvp3Bc
LvL+InjCSD6bPrvYNtatGF76Xv7BGiyTnwKXISQQOXS/XTQEXduKtBzgPg+waR1U
AMHjrF2McEyVm0yBHlwc0wO8gH3oWoGHddy/OdQJhAGu5F2EXmOj7S2rWhnGWYtu
WBirsBbAnbxE0K8kAwAk/j/kr4YGwGMDOpYJGXV4dsTnOCc=
-----END CERTIFICATE-----`

	svrKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA483dAGr4vGI2SNjVdebGtyV4b7h1cA6zrhzvtrgGg4/OYHyD
04BA43flZ26rIjdkXyoSNWe5e+RNyqJ/K/afL5JfWzei8EpZkbtlhZenDPA/MSQK
V1GSeWjPRkwsEqMbdJeIcQwmhdIaxcXaNDXS40WcRXZydcgXD9X54WZ2c7Nr6fG3
RJ6zVi0tBipCLa5mCz5KVKMkktwD9yq7C4AfchabG24xBcR9rcD4issUPL+Tizlj
mbxSwKj1bqo/4A920V6FzUWjkDMJSA1lhfLsU+nqlc8cK349dxPWCAdSvKxoP4A1
gMO5oz15Tp6OZQIbV1jwMkkJjFoZ8YGiYtpAawIDAQABAoIBAQCCS+Rlccn1jkM4
ZXWqqyXr6W26kQny3yXcp8Zgf1+SbnV/cJjCJ3B16sT25TDTMFWjrN+fVkWcXFg2
V71fev9P6WvLM8ZppE0Y8tO9lqFA4EV0qQWVLh4WfWFY9waaXlq81FOBPY7nKeaQ
SntlM4f84HrirD4JqjmuoBf92WpVAC1xLIBmmLk3+td1+SWzDwXMTNqWBd0NOWNg
ezI/f9k0e2HM86pv2Vop/GIuhsLdADmRtVmU+vypJ7fCBKzszDa+urSsS8R9Pf40
wXertEAX6iCGMbAV0U7ZjlMgKf7LtLzfGtq+FCFDtznWXf3RqZxF7Q63UPM55VIu
1gTh6E9ZAoGBAPvRqTfiSLHg+6b7UFWJxUKbgw3f6eviklP2cq1xi4HYGfZDBngY
e89HOOLDj2eTzfANqLFi5DZzMxIzURrylpbwdVONOD4LWnwgA5Hp1O2esksyl+9h
9I/LGh602y/vekYvlHsJbp3U2S9Gwss8h/+KW2Nz3uCspyvfyrOq+k+FAoGBAOeW
IP/Pb0vU+3BHQo2RGrwHw1Rr0J5SuNoz2oQgr2wUTCLdrlzubsTKs5LTPVTSZBMx
TUf7DkUnsrs4Xh3O46DC+z5fU/WZQzP61jpC58AbxdlB9/QLj86AapZ3HxzQb6e4
rxdYrtg1uxnIpY5ehZwYGs8yuLwqHXtQUXvT6jsvAoGBAPsO5dHEdct6TgsFxery
B0vH9ZoQoow9gLvbGiwX5wmWJRQjcMCtUEqwbGOQq1mNv6TUSVpJCNPMeJ9tsC/Y
qhBkPeUGB4u8EANue4CvC024iXN1RoswMv5ldG4my9x3uoVdDIC6P6F1wu5icvTj
LYe1LjXyIMQI/kY8wT/td89tAoGASlsHiVrezyg4+tnGYpG+VbTgYFClkM/ajiSr
+lRMPpVdxKwMecYMRp8WfQPZ40wR2Z+wwnW3JTkTx8zXWxa8OzefV21gFbD5xMy6
z8X/hszj/1eQ9whnSdQtZNYmZSf/UYiYnxRYPw8xXZvwm/95Qp7yrKgKbE/RW3B0
WR+3Sv0CgYBMDpsjzASGXPZlkTYbDsL5alzckSZwyA9vSxRtnVvEnOIGUErs/Hv3
YLBV7sNcmwnt3m6sBAkR6gNxgjEzUS2zC+nfKgLU1YbD3KLuPCpkNgHiXKsvQIVV
sLoII402XJ/WfXir6sWi9pjJyTFi03gXltq17WQspd3DpqHp2G34gw==
-----END RSA PRIVATE KEY-----`
)

var (
	caFile  = "ca.crt"
	crtFile = "server.crt"
	keyFile = "server.key"
)

func main() {
	caBytes := []byte(caCrt)
	crtBytes := []byte(svrCrt)
	keyBytes := []byte(svrKey)

	err := ioutil.WriteFile(caFile, caBytes, 0644)
	if err != nil {
		log.Errf("Error writing file: %s", caFile)
		os.Exit(1)
	}
	err = ioutil.WriteFile(crtFile, crtBytes, 0644)
	if err != nil {
		log.Errf("Error writing file: %s", crtFile)
		os.Exit(1)
	}
	err = ioutil.WriteFile(keyFile, keyBytes, 0644)
	if err != nil {
		log.Errf("Error writing file: %s", keyFile)
		os.Exit(1)
	}
}
