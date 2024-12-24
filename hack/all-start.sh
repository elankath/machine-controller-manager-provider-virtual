#!/usr/bin/env zsh
script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/init.sh"
source "$script_dir/helper/kvcl.sh"

set -eo pipefail

declare pids=()
cleanup() {
  set +e
  for pid in "${pids[@]}"; do
    if kill -TERM $pid 2>/dev/null; then
      kill $pid
    fi
  done
  kill_by_name kvcl
  kill_by_name kube-apiserver
  kill_by_name etcd
  kill_by_name cluster-autoscaler
  kill_by_name machine-controller-manager
  kill_by_name machine-controller
  if [[ -f "$kvcl_pid_path" ]]; then
    rm "$kvcl_pid_path"
  fi
}

main() {
  echo "NOTE: This is temporary working solution - START command will be delegated to Go HACK binary in future for reliability."
  trap cleanup EXIT ERR SIGINT SIGTERM
  sleep 2
#  start_kvcl_script="./hack/start-kvcl.sh"
#  "$start_kvcl_script" &
#  pids+=($!)
  echo ">> INVOKING START KVCL (virtual cluster: api-server+scheduler) ...."
 ./hack/start-kvcl.sh &
  pids+=($!)
  echo "Waiting some seconds..."
  sleep 7

  echo ">> INVOKING START MCM (machine-contorller-manager) ...."
 ./hack/start-mcm.sh &
  pids+=($!)
  echo "Waiting some seconds..."
  sleep 3

  echo ">> INVOKING START MC (machine-controller) (machin-controller-manager-provider-virtual) ...."
 ./hack/start-mc.sh &
  pids+=($!)
  echo "Waiting some seconds..."
  sleep 3

  echo ">> INVOKING CA (cluster-autoscaler) START...."
 ./hack/start-ca.sh &
  pids+=($!)
  echo "Waiting some seconds..."
  sleep 3

  echo "> Press Ctrl-C to abort."
  tail -f /dev/null #sleep forever
}

main