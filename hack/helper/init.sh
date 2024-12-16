script_helper_dir=$(dirname "${(%):-%x}")
source "$script_helper_dir/common.sh"

init_local_cluster() {
  set -o pipefail
  echo "init_local_cluster commencing."
  local cmd

  exists_file_or_exit "$init_env_path" "init_env_path: $init_env_path does not exist. Kindly run ./hack/setup.sh first before launch" 11
  source "$init_env_path"

  echo "Using KUBECONFIG: $KUBECONFIG"
  export KUBECONFIG="$KUBECONFIG"

  check_set_mcm_dir

  cmd="kubectl apply -f $MCM_CRD_DIR"
  echo "Applying CRDs using: ${cmd}"
  eval "$cmd"

  exists_file_or_exit "$init_mcc_path" "init_mcc_path: $init_init_mcc_path does not exist. Kindly run ./hack/setup.sh first before launch" 11
  exists_file_or_exit "$init_mcd_path" "init_mcd_path: $init_init_mcd_path does not exist. Kindly run ./hack/setup.sh first before launch" 11

  source "$init_env_path"
  if [[ -z "$SHOOT_NAMESPACE" ]]; then
    error_exit "SHOOT_NAMESPACE env not set. Kindly run ./hack/setup.sh before invoking init_local_cluster"
      return
  fi

  cmd="kubectl create ns $SHOOT_NAMESPACE"
  echo "Creating SHOOT_NAMESPACE: $SHOOT_NAMESPACE: $cmd"
  eval "$cmd"
  if [[ $? -ne 0 ]]; then
      error_exit "The creation of shoot namespace failed with exit code $?" "$?"
      return
  else
      echo "SHOOT_NAMESPACE: $SHOOT_NAMESPACE created successfully on virtual cluster"
  fi

  cmd="kubectl apply -f $init_mcc_path"
  echo "Applying MachineClasses using: ${cmd}"
  eval "$cmd"
  if [[ $? -ne 0 ]]; then
      error_exit "The creation of MachineClasses failed with exit code $?" "$?"
      return
  else
      echo "MachinClasses at $init_mcc_path created successfully on virtual cluster"
  fi

  cmd="kubectl apply -f $init_mcd_path"
  echo "Applying MachineDeployments using: ${cmd}"
  eval "$cmd"
  if [[ $? -ne 0 ]]; then
      error_exit "The creation of MachineDeployments  failed with exit code $?" "$?"
      return
  else
      echo "MachineDeployments at $init_mcd_path created successfully on virtual cluster"
  fi



  echo "init_local_cluster completed."
}
