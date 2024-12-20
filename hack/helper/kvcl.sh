#!/usr/bin/env zsh

declare VERBOSE=true

heler_script_dir=$(dirname "${(%):-%x}")
source "$heler_script_dir/common.sh"

# Define cleanup function
cleanup_kvcl() {
  set +e
  kill_by_name kvcl
  kill_by_name kube-apiserver
  kill_by_name etcd
  if [[ -f "$kvcl_pid_path" ]]; then
    rm "$kvcl_pid_path"
  fi
}

kvcl_launch() {
  set -o pipefail

  # Set trap for script termination
  trap cleanup_kvcl EXIT ERR SIGINT SIGTERM
  echo "Binaries Path: $bin_dir"
  kvcl_bin="${bin_dir}/kvcl"
  export BINARY_ASSETS_DIR="$bin_dir"
  kvcl_bin="${bin_dir}/kvcl"
  exists_file_or_exit "$kvcl_bin" "KVCL Binary is not present at $kvcl_bin. Kindly first run ./hack/setup.sh" 1
  cleanup_kvcl
  echo "Launching kvcl via $kvcl_bin..."
  "$kvcl_bin" 2>&1 | tee "$kvcl_log_file" &
  local kvcl_pid="$!"
  echo "$kvcl_pid" > "$kvcl_pid_path"
  sleep 6
  echo "NOTE: kvcl LAUNCHED with PID $kvcl_pid written to $kvcl_pid_path. Logs output to $kvcl_log_file. KUBECONFIG=$KUBECONFIG"
  tail -f /dev/null #sleep forever
}



