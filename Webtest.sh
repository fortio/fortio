#! /bin/bash
set -x
# Check we can build the image
make docker-internal TAG=webtest || exit 1
DOCKERID=$(docker run -d -p 8088:8080 istio/fortio:webtest server)
function cleanup {
  docker stop $DOCKERID
}
trap cleanup EXIT
set -e
set -o pipefail
sleep 10
docker ps
BASE_URL="http://localhost:8088"
# Check we can connect, and run a QPS test against ourselves through fetch
curl -f "$BASE_URL/fortio/fetch/localhost:8080/fortio/?url=http://localhost:8080/debug&load=Start&qps=-1&json=on" | grep ActualQPS
# Check we get the logo
LOGO_TYPE=$(curl -o /dev/null -f -w '%{content_type}' "$BASE_URL/fortio/logo.svg")
if [ "$LOGO_TYPE" != "image/svg+xml" ]; then
  echo "Unexpected content type for the logo: $LOGO_TYPE"
  exit 1
fi
# Check we can get the JS file through the proxy and it's > 50k
SIZE=$(curl -v -f "$BASE_URL/fortio/fetch/localhost:8080/fortio/Chart.min.js" |wc -c)
if [ "$SIZE" -lt 50000 ]; then
  echo "Too small fetch for js: $SIZE"
  exit 1
fi
