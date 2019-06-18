#!/usr/bin/env bash

# This entrypoint will wait for istio-proxy to be available

echo "Wait man"

count=30
until curl -I localhost:15000; do
	echo Waiting for Sidecar
	count=$((count - 1))
	sleep 2

	if [ $count -lt 0 ]; then
		echo "Bypassing sidecar check, took too long (60s++)"
		break
	fi
done

echo Starting fortio "$@" in 10s
sleep 10 # Sometimes still need a few seconds after its "Ready"
/usr/bin/fortio "$@"
