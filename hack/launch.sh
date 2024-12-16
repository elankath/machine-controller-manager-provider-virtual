#!/usr/bin/env zsh


script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/kvcl.sh"
source "$script_dir/helper/init.sh"


bin_path=$(realpath "${script_dir}/../bin")
mcm_bin="${bin_path}/machine-controller-manager"
exists_file_or_exit "$mcm_bin" "MCM Binary is not present at $mcm_bin. Kindly first run ./hack/setup.sh" 2

declare mcm_log_file="/tmp/mcm.log"
declare kubeconfig_path="/tmp/kvcl.yaml"
declare kvcl_launch_wait="7"



main() {
  set -eo pipefail
  trap cleanup_kvcl EXIT ERR SIGINT SIGTERM
  kvcl_launch &
  echo "Waiting for $kvcl_launch_wait secs for KVCL to boot up.."
  sleep "$kvcl_launch_wait"
  exists_file_or_exit "$kubeconfig_path" "KVCL did not produce KUBECONFIG at expected path $kubeconfig_path. Kindly check $kvcl_log_file." 3

  export KUBECONFIG="$kubeconfig_path"
  init_local_cluster
  echo "Launching MCM (machine-controller-manager)..."
  "$mcm_bin" --control-kubeconfig="$kubeconfig_path" \
    --target-kubeconfig="$kubeconfig_path" \
    --namespace="$SHOOT_NAMESPACE" \
    --leader-elect=false \
      2>&1 | tee "$mcm_log_file"
#  "$mcm_bin" > "$mcm_log_file" 2>&1 &
#  mcm_pid="$!"
#  echo "MCM has been launched in background with PID: $mcm_pid with logs at $mcm_log_file"

}

# Run main function
main "$@"