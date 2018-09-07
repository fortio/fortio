#!/bin/sh
set -e
set -u

VERSION=`/source/usr/local/bin/fortio version | cut -f 1 -d ' '`
echo $VERSION
# how that we have our version, let's build DEB
cd /source
fpm -s dir -t deb -n fortio -v $VERSION .
cp fortio*.deb /packages
