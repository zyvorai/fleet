#!/bin/sh
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
set -eu
cd "$(dirname "$0")/.."
test -z "$(gofmt -l cmd internal webui)"
go vet ./...
go test -race ./...
node --check webui/static/app.js
make build
printf '%s\n' 'All Zyvor Fleet checks passed.'
