#!/bin/bash
# Securexe stages the built binary in as "private-file-share-bin" next to
# this script. Bare invocation opens the browser UI directly — no
# terminal usage dump — so this just execs straight through.
cd "$(dirname "$0")"
exec ./private-file-share-bin
