## gRPC TLS Generation

Fortio provides an optional gRPC client and server for load testing gRPC.
When gRPC mutual TLS (mTLS) is enabled using the `-grpc-secure true` flag, TLS credentials are used to create a secure
connection between the client and server. The Fortio server requires a TLS server certificate, key, and CA certificate.
The Fortio gRPC client (`grpcping`) requires a TLS client certificate, key, and CA certificate.

The `cert-gen` script generates a self-signed CA, client/server certificates and keys.

**Note:** Prefer your organization's PKI, if possible

Navigate to the `tools` directory.

```sh
$ cd tools
```

Export `SAN` to set the Subject Alt Names used in certificates. Provide the fully qualified domain name
or IP (discouraged) where Fortio will be installed.

```sh
# DNS or IP Subject Alt Names where fortio runs
$ export SAN=DNS.1:fortio.example.com,IP.1:172.16.0.2
```

Generate a `ca.crt`, `server.crt`, `server.key`, `client.crt`, and `client.key`.

```sh
$ ./cert-gen
Creating FAKE CA, server cert/key, and client cert/key...
...
...
...
******************************************************************
WARNING: Generated credentials are self-signed. Prefer your
organization's PKI for production deployments.
```

Move TLS credentials to the fortio's default path.

```sh
$ sudo mkdir -p /etc/ssl/certs/
$ sudo cp ca.crt client.crt server.crt client.key server.key /etc/ssl/certs/
```

## Inpsect

Inspect the generated certificates if desired.

```sh
openssl x509 -noout -text -in ca.crt
openssl x509 -noout -text -in server.crt
openssl x509 -noout -text -in client.crt
```

## Verify

Verify that the server and client certificates were signed by the self-signed CA.

```sh
openssl verify -CAfile ca.crt server.crt
openssl verify -CAfile ca.crt client.crt
```

## Example Usage

* Start the Fortio server with mTLS enabled:
```
$ fortio server -grpc-secure &
Https redirector running on :8081
UI starting - visit:
http://localhost:8080/fortio/   (or any host/ip reachable on this server)
Fortio 0.7.3 grpc ping server listening on port :8079
Fortio 0.7.3 echo server listening on port :8080
Using TLS server certificate: /etc/ssl/certs/server.crt
Using TLS server key: /etc/ssl/certs/server.key
Using CA certificate: /etc/ssl/certs/ca.crt to authenticate server certificate
```
* Start a grpc ping with mTLS enabled:
```
$ fortio grpcping -grpc-secure localhost
Using TLS client certificate: /etc/ssl/certs/client.crt
Using TLS client key: /etc/ssl/certs/client.key
Using CA certificate: /etc/ssl/certs/ca.crt to authenticate server certificate
02:29:27 I pingsrv.go:116> Ping RTT 305334 (avg of 342970, 293515, 279517 ns) clock skew -2137
Clock skew histogram usec : count 1 avg -2.137 +/- 0 min -2.137 max -2.137 sum -2.137
# range, mid point, percentile, count
>= -4 < -2 , -3 , 100.00, 1
# target 50% -2.137
RTT histogram usec : count 3 avg 305.334 +/- 27.22 min 279.517 max 342.97 sum 916.002
# range, mid point, percentile, count
>= 250 < 300 , 275 , 66.67, 2
>= 300 < 350 , 325 , 100.00, 1
# target 50% 294.879
```
