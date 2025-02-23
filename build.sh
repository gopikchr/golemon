#!/usr/bin/env bash
set -euo pipefail

go build -o bin/golemon .
cc -o bin/lemonc ./intermediate/lemon.c
cp intermediate/lempar.c bin/lempar.c
cp lempar.go.tpl bin/lempar.go.tpl
