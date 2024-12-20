#!/usr/bin/env zsh


script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/kvcl.sh"
source "$script_dir/helper/init.sh"

declare kubeconfig_path="/tmp/kvcl.yaml"
declare kvcl_launch_wait="7"

main() {
  set -eo pipefail
  kvcl_launch
}

# Run main function
main "$@"