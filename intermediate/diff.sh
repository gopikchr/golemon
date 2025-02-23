#!/usr/bin/env bash
set -euo pipefail

echo "diff -b lemon.c ../../sqlite/tool/lemon.c"
diff -b lemon.c ../../sqlite/tool/lemon.c
echo "diff -b lempar.c ../../sqlite/tool/lempar.c"
diff -b lempar.c ../../sqlite/tool/lempar.c
