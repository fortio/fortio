#! /bin/bash
set -x
# Check we can build the image
make docker-internal TAG=webtest || exit 1
FORTIO_UI_PREFIX=/newprefix/ # test the non default prefix (not /fortio/)
DOCKERID=$(docker run -d --name fortio_server istio/fortio:webtest server -ui-path $FORTIO_UI_PREFIX)
function cleanup {
  docker stop $DOCKERID
  docker rm fortio_server
}
trap cleanup EXIT
set -e
set -o pipefail
docker ps
BASE_URL="http://localhost:8080"
BASE_FORTIO="$BASE_URL$FORTIO_UI_PREFIX"
# Check https works (certs are in the image)
docker exec fortio_server /usr/local/bin/fortio load -curl -stdclient https://istio.io/robots.txt
CURL="docker exec fortio_server /usr/local/bin/fortio load -curl"
# Check we can connect, and run a QPS test against ourselves through fetch
$CURL "${BASE_FORTIO}fetch/localhost:8080$FORTIO_UI_PREFIX?url=http://localhost:8080/debug&load=Start&qps=-1&json=on" | grep ActualQPS
# Check we get the logo (need to remove the CR from raw headers)
LOGO_TYPE=$($CURL "${BASE_FORTIO}static/img/logo.svg" | grep -i Content-Type: | tr -d '\r'| awk '{print $2}')
if [ "$LOGO_TYPE" != "image/svg+xml" ]; then
  echo "Unexpected content type for the logo: $LOGO_TYPE"
  exit 1
fi
# Check we can get the JS file through the proxy and it's > 50k
SIZE=$($CURL "${BASE_FORTIO}fetch/localhost:8080${FORTIO_UI_PREFIX}static/js/Chart.min.js" |wc -c)
if [ "$SIZE" -lt 50000 ]; then
  echo "Too small fetch for js: $SIZE"
  exit 1
fi
