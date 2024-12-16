#!/usr/bin/env zsh

declare VERBOSE=true

heler_script_dir=$(dirname "${(%):-%x}")
source "$heler_script_dir/common.sh"


# Define cleanup function
cleanup_kvcl() {
  for p in $(pgrep -f envtest); do kill -9 $p;done
  for p in $(pgrep -f kube-apiserver); do kill -9 $p;done
  for p in $(pgrep -f etcd); do kill -9 $p;done
}

kvcl_launch() {
  set -eo pipefail

  # Set trap for script termination
  trap cleanup_kvcl EXIT ERR SIGINT SIGTERM
  declare kvcl_log_file="/tmp/kvcl.log"
  bin_path=$(realpath "${heler_script_dir:A}/../../bin")
  echo "Binaries Path: $bin_path"
  kvcl_bin="${bin_path}/kvcl"
  export BINARY_ASSETS_DIR="$bin_path"
  kvcl_bin="${bin_path}/kvcl"
  exists_file_or_exit "$kvcl_bin" "KVCL Binary is not present at $kvcl_bin. Kindly first run ./hack/setup.sh" 1

  cleanup_kvcl
  echo "Launching k8s Virtual Cluster via $kvcl_bin..."
  "$kvcl_bin" 2>&1 | tee "$kvcl_log_file"
}



