#! /bin/bash
set -x
# Check we can build the image
make docker-internal TAG=webtest || exit 1
DOCKERID=$(docker run -d --name fortio_server istio/fortio:webtest server)
function cleanup {
  docker stop $DOCKERID
  docker rm fortio_server
}
trap cleanup EXIT
set -e
set -o pipefail
docker ps
BASE_URL="http://localhost:8080"
# Check https works (certs are in the image)
docker exec fortio_server /usr/local/bin/fortio load -curl -stdclient https://istio.io/robots.txt
# Needed for circleci docker environment
CURL="docker run --network container:fortio_server appropriate/curl --retry 10 --retry-connrefused"
# Check we can connect, and run a QPS test against ourselves through fetch
$CURL -f "$BASE_URL/fortio/fetch/localhost:8080/fortio/?url=http://localhost:8080/debug&load=Start&qps=-1&json=on" | grep ActualQPS
# Check we get the logo
LOGO_TYPE=$($CURL -o /dev/null -f -w '%{content_type}' "$BASE_URL/fortio/static/img/logo.svg")
if [ "$LOGO_TYPE" != "image/svg+xml" ]; then
  echo "Unexpected content type for the logo: $LOGO_TYPE"
  exit 1
fi
# Check we can get the JS file through the proxy and it's > 50k
SIZE=$($CURL -v -f "$BASE_URL/fortio/fetch/localhost:8080/fortio/static/js/Chart.min.js" |wc -c)
if [ "$SIZE" -lt 50000 ]; then
  echo "Too small fetch for js: $SIZE"
  exit 1
fi
