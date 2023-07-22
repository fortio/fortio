#!/usr/bin/env bash

set -e
echo "" > coverage.txt
rm -f profile.out

for d in $(go list ./... | grep -v vendor); do
    echo "### Working on package coverage $d"
    go test -v -coverprofile=profile.out -covermode=atomic $d
    if [ -f profile.out ]; then
        cat profile.out >> coverage.txt
        rm profile.out
    fi
done
