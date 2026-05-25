#!/usr/bin/env bash
# ---
# description: Print a hello message
# usage: hello.sh --name {{name}}
# type:
#   input: [text]
#   output: [text]
# arguments:
#   name:
#     pattern: '(?i)(?:name)\s+(\S+)'
#     description: Name to greet
# exits:
#   0: success
# ---

NAME="${1:-World}"
echo "Hello, $NAME!"
