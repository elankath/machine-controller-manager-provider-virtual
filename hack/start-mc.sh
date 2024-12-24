#!/usr/bin/env zsh


script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/kvcl.sh"
source "$script_dir/helper/init.sh"


exists_file_or_exit "$mc_bin_path" "MC Binary is not present at $mc_bin_path. Kindly first run ./hack/setup.sh" 2

verify() {
  check_kvcl_running
  check_mc_binary

}

main() {
  set -eo pipefail

  verify

  export KUBECONFIG="$local_kubeconfig"
  init_local_cluster
  echo "Launching MC (machine-controller-manager-provider-virtual)..."
  "$mc_bin_path" --control-kubeconfig="$KUBECONFIG" \
    --target-kubeconfig="$KUBECONFIG" \
    --namespace="$SHOOT_NAMESPACE" \
    --leader-elect=false | tee /tmp/mc.log

}

# Run main function
main "$@"