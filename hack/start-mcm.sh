#!/usr/bin/env zsh


script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/kvcl.sh"
source "$script_dir/helper/init.sh"


exists_file_or_exit "$mcm_bin_path" "MCM Binary is not present at $mcm_bin_path. Kindly first run ./hack/setup.sh" 2
declare kvcl_launch_wait="7"

verify() {
  check_kvcl_running
  check_mcm_binary

}

main() {
  set -eo pipefail

  verify

  export KUBECONFIG="$local_kubeconfig"
  init_local_cluster
  echo "Launching MCM (machine-controller-manager)..."
  "$mcm_bin_path" --control-kubeconfig="$KUBECONFIG" \
    --target-kubeconfig="$KUBECONFIG" \
    --namespace="$SHOOT_NAMESPACE" \
    --leader-elect=false 2>&1 | tee /tmp/mcm.log

}

# Run main function
main "$@"