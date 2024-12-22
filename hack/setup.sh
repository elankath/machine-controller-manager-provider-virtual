#!/usr/bin/env zsh
set -eo pipefail
script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"

echo "Building hack..."
go -C "$script_dir" build -o "${hack_bin_path}" main.go
echo "Running hack setup $*"
$hack_bin_path setup "$@"
