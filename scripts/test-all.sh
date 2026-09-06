#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
test -z "$(gofmt -l cmd internal webui)"
go vet ./...
go test -race ./...
node --check webui/static/app.js
make build
printf '%s\n' 'All Zyvor Fleet checks passed.'
