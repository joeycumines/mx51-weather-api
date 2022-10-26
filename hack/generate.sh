#!/usr/bin/env bash

# (re)generates all auto-generated source and specifications

set -euo pipefail || exit 1
trap 'echo [FAILURE] line_number=${LINENO} exit_code=${?} bash_version=${BASH_VERSION}' ERR
script_path="$( (DIR="$( (
    SOURCE="${BASH_SOURCE[0]}"
    while [ -h "$SOURCE" ]; do
        DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
        SOURCE="$(readlink "$SOURCE")"
        [[ ${SOURCE} != /* ]] &&
            SOURCE="$DIR/$SOURCE"
    done
    echo "$(cd -P "$(dirname "$SOURCE")" && pwd)"
))" &&
    [ ! -z "$DIR" ] &&
    cd "$DIR" &&
    pwd))"

command -v protoc >/dev/null 2>&1
command -v protoc-gen-go >/dev/null 2>&1

# shellcheck disable=SC2054
cmd=(
    protoc
    --go_out=. --go_opt=paths=source_relative
)

cd "$script_path/.."
echo "project path: $(pwd)"

find . \
    -not \( -path ./hack -prune \) \
    -type f \
    -name '*.proto' \
    -exec "${cmd[@]}" {} +
