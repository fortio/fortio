#! /bin/bash
# Copyright 2017 Istio Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
set -x
# Check we can build the image
NATIVE_PLATFORM=$(docker buildx --builder default inspect | tail -1 | sed -e "s/Platforms: //" -e "s/,//g" | awk '{print $1}')
echo "Building for $NATIVE_PLATFORM"
make docker-internal TAG=webtest BUILDX_PLATFORMS="$NATIVE_PLATFORM" MODE=dev || exit 1
FORTIO_UI_PREFIX=/newprefix/ # test the non default prefix (not /fortio/)
FILE_LIMIT=25 # must be low to detect leaks, go 1.14 seems to need more than go1.8 (!)
LOGLEVEL=info # change to debug to debug
MAXPAYLOAD=8 # Max Payload size for echo?size= in kb
TIMEOUT=10s # need to be higher than test duration done through fetch
#CERT=/etc/ssl/certs/ca-certificates.crt
TEST_CERT_VOL=/etc/ssl/certs/fortio
DOCKERNAME=fortio_server
DOCKERSECNAME=fortio_secure_server
DOCKERSECVOLNAME=fortio_certs
FORTIO_BIN_PATH=fortio # /usr/bin/fortio is the full path but isn't needed

# Unresolvable should error out - #653
docker run fortio/fortio:webtest curl http://doesnt.exist.google.com/
if [[ $? == 0 ]]; then
  echo "Error in curl should show up in status"
  exit 1
fi

# Expect error with extra args: (timeout (brew install coreutils) returns 124
# for timeout) - #652
timeout 3 docker run fortio/fortio:webtest server -loglevel debug extra-arg
if [[ $? == 124 || $? == 0 ]]; then
  echo "Unrecognized extra args/typo in flags should error out immediatly"
  exit 1
fi

DOCKERID=$(docker run -d --ulimit nofile=$FILE_LIMIT --net host --name $DOCKERNAME fortio/fortio:webtest server -ui-path $FORTIO_UI_PREFIX -loglevel $LOGLEVEL -maxpayloadsizekb $MAXPAYLOAD -timeout=$TIMEOUT)
function cleanup {
  set +e # errors are ok during cleanup
#  docker logs "$DOCKERID" # uncomment to debug
  docker stop "$DOCKERID"
  docker rm -f $DOCKERNAME
  docker stop "$DOCKERSECID" # may not be set yet, it's ok
  docker rm -f $DOCKERSECNAME
  docker stop "$DOCKERCURLID"
  docker rm -f $DOCKERSECVOLNAME
}
trap cleanup EXIT
set -e
set -o pipefail
docker ps
BASE_URL="http://localhost:8080"
BASE_FORTIO="$BASE_URL$FORTIO_UI_PREFIX"
LOGO=fortio-logo-gradient-no-bg.svg
CURL="docker exec $DOCKERNAME $FORTIO_BIN_PATH curl -loglevel $LOGLEVEL -timeout $TIMEOUT"
# Check https works (certs are in the image) - also tests autoswitch to std client for https
$CURL https://www.google.com/robots.txt > /dev/null

# Check that quiet is quiet. Issue #385.
QUIETCURLTEST="docker exec $DOCKERNAME $FORTIO_BIN_PATH curl -quiet -curl-stdout-headers www.google.com"
if [ "$($QUIETCURLTEST 2>&1 > /dev/null  | wc -l)" -ne 0 ]; then
  echo "Error, -quiet still outputs logs"
  $QUIETCURLTEST > /dev/null
  exit 1
fi

# Check we can connect, and run a http QPS test against ourselves through fetch
$CURL "${BASE_FORTIO}fetch/localhost:8080$FORTIO_UI_PREFIX?url=http://localhost:8080/debug&load=Start&qps=-1&json=on" | grep ActualQPS
# Check we can do it twice despite ulimit - check we get all 200s (exactly 80 of them (default is 8 connections->16 fds + a few))
$CURL "${BASE_FORTIO}fetch/localhost:8080$FORTIO_UI_PREFIX?url=http://localhost:8080/debug&load=Start&n=80&qps=-1&json=on" | grep '"200": 80'
# Same but using the rest api instead
$CURL "${BASE_FORTIO}fetch/localhost:8080${FORTIO_UI_PREFIX}rest/run?url=http://localhost:8080/debug&n=80&qps=-1" | grep '"200": 80'
# Check we can connect, and run a grpc QPS test against ourselves through fetch
$CURL "${BASE_FORTIO}fetch/localhost:8080$FORTIO_UI_PREFIX?url=localhost:8079&load=Start&qps=-1&json=on&n=100&runner=grpc" | grep '"SERVING": 100'
# Check we get the logo (need to remove the CR from raw headers)
VERSION=$(docker exec $DOCKERNAME $FORTIO_BIN_PATH version -s)
LOGO_TYPE=$($CURL "${BASE_FORTIO}${VERSION}/static/img/${LOGO}" 2>&1 >/dev/null | grep -i Content-Type: | tr -d '\r'| awk '{print $2}')
if [ "$LOGO_TYPE" != "image/svg+xml" ]; then
  echo "Unexpected content type for the logo: $LOGO_TYPE"
  exit 1
fi
# Check we can get the JS file through the proxy and it's > 50k
SIZE=$($CURL "${BASE_FORTIO}fetch/localhost:8080${FORTIO_UI_PREFIX}${VERSION}/static/js/Chart.min.js" |wc -c)
if [ "$SIZE" -lt 50000 ]; then
  echo "Too small fetch for js: $SIZE"
  exit 1
fi
# Check if max payload set to value passed in cmd line parameter -maxpayloadsizekb
SIZE=$($CURL "${BASE_URL}/echo?size=1048576" |wc -c)
# Payload is 8192 but between content chunking and headers fast client can return up to 8300 or so
if [ "$SIZE" -lt 8191 ] || [ "$SIZE" -gt 8400 ]; then
  echo "-maxpayloadsizekb not working as expected"
  exit 1
fi

# Check main, sync, browse pages
VERSION=$(docker exec $DOCKERNAME $FORTIO_BIN_PATH version -s)
LOGOPATH="${VERSION}/static/img/${LOGO}"
for p in "" browse sync; do
  # Check the page doesn't 404s
  $CURL ${BASE_FORTIO}${p}
  # Check that page includes the logo
  LOGOS=$($CURL ${BASE_FORTIO}${p} | { grep -c "$LOGOPATH" || true; })
  if [ "$LOGOS" -ne 1 ]; then
    echo "*** Expected to find logo $LOGOPATH in the ${BASE_FORTIO}${p} page"
    exit 1
  fi
done

# Do a small http load using std client
docker exec $DOCKERNAME $FORTIO_BIN_PATH load -stdclient -qps 1 -t 2s -c 1 https://www.google.com/
# and with normal and with custom headers
docker exec $DOCKERNAME $FORTIO_BIN_PATH load -H Foo:Bar -H Blah:Blah -qps 1 -t 2s -c 2 http://www.google.com/
# Do a grpcping
docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping localhost
# Do a grpcping to a scheme-prefixed destination. Fortio should append port number
# Do a TLS grpcping. Fortio.org should use valid cert.
docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping https://grpc.fortio.org
docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping grpc.fortio.org # uses default non tls 8079
# Do a local grpcping. Fortio should append default grpc port number to destination
docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping localhost
# Do a local health ping
docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping -health localhost
docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping -health -healthservice ping localhost
# Do a failing on purpose check
if docker exec $DOCKERNAME $FORTIO_BIN_PATH grpcping -health -healthservice ping_down localhost; then
  echo "*** Expecting grpcping -health to ping_down have exit with error/non zero status"
  exit 1
fi
# pprof should be there, no 404/error
PPROF_URL="$BASE_URL/debug/pprof/heap?debug=1"
$CURL "$PPROF_URL" | grep -i TotalAlloc # should find this in memory profile
# creating dummy container to hold a volume for test certs due to remote docker bind mount limitation.
DOCKERCURLID=$(docker run -d -v $TEST_CERT_VOL --net host --name $DOCKERSECVOLNAME docker.io/fortio/fortio.build:v54@sha256:4775038c3ace753978c8dfd99ebbd23607b61eeb0bf6c2bf2901d0485cc1870c sleep 120)
# while we have something with actual curl binary do
# Test for h2c upgrade (#562)
docker exec $DOCKERSECVOLNAME /usr/bin/curl -v --http2 -m 10 -d foo42 http://localhost:8080/debug | tee >(cat 1>&2) | grep foo42
# then resume the self signed CA tests
# copying cert files into the certs volume of the dummy container
for f in ca.crt server.crt server.key; do docker cp "$PWD/cert-tmp/$f" "$DOCKERSECVOLNAME:$TEST_CERT_VOL/$f"; done
# start server in secure grpc mode. uses non-default ports to avoid conflicts with fortio_server container.
# mounts certs volume from dummy container.
DOCKERSECID=$(docker run -d --ulimit nofile=$FILE_LIMIT --name $DOCKERSECNAME --volumes-from $DOCKERSECVOLNAME fortio/fortio:webtest server -cacert $TEST_CERT_VOL/ca.crt -cert $TEST_CERT_VOL/server.crt -key $TEST_CERT_VOL/server.key -grpc-port 8097 -http-port 8098 -redirect-port 8090 -loglevel $LOGLEVEL)
# run secure grpcping and load tests
docker exec $DOCKERSECNAME $FORTIO_BIN_PATH grpcping -cacert $TEST_CERT_VOL/ca.crt localhost:8097
docker exec $DOCKERSECNAME $FORTIO_BIN_PATH load -grpc -cacert $TEST_CERT_VOL/ca.crt localhost:8097
# switch to report mode
docker stop "$DOCKERID"
docker rm $DOCKERNAME
DOCKERNAME=fortio_report
DOCKERID=$(docker run -d --ulimit nofile=$FILE_LIMIT --name $DOCKERNAME fortio/fortio:webtest report -loglevel $LOGLEVEL)
docker ps
CURL="docker exec $DOCKERNAME $FORTIO_BIN_PATH curl -loglevel $LOGLEVEL"
if $CURL "$PPROF_URL" ; then
  echo "pprof should 404 on report mode!"
  exit 1
else
  echo "expected pprof failure to access in report mode - good !"
fi
# base url should serve report only UI in report mode
$CURL $BASE_URL | grep "report only limited UI"
# we should get the tsv without error
$CURL $BASE_URL/data/index.tsv
# cleanup() will clean everything left even on success
