script_helper_dir=$(dirname "${(%):-%x}")
source "$script_helper_dir/common.sh"

init_local_cluster() {
  set -o pipefail
  echo "init_local_cluster commencing."
  local cmd
  local cmdOut

  exists_file_or_exit "$env_path" "init_local_cluster: $env_path does not exist. Kindly run ./hack/setup.sh first before launch" 11
  source "$env_path"

  echo "Using KUBECONFIG: $KUBECONFIG"
  export KUBECONFIG="$local_kubeconfig"

  check_set_mcm_dir

  cmd="kubectl apply -f $MCM_CRD_DIR"
  echo "Applying CRDs using: ${cmd}"
  eval "$cmd"

  exists_file_or_exit "$mcc_path" "init_local_cluster: $mcc_path does not exist. Kindly run ./hack/setup.sh first before launch" 11
  exists_file_or_exit "$mcd_path" "init_local_cluster: $mcd_path does not exist. Kindly run ./hack/setup.sh first before launch" 11

  source "$env_path"
  if [[ -z "$SHOOT_NAMESPACE" ]]; then
    error_exit "SHOOT_NAMESPACE env not set. Kindly run ./hack/setup.sh before invoking init_local_cluster"
    return
  fi

  if ! kubectl get namespace "$SHOOT_NAMESPACE" >/dev/null 2>&1; then
    cmd="kubectl create ns $SHOOT_NAMESPACE"
    echo "Creating SHOOT_NAMESPACE: $SHOOT_NAMESPACE: $cmd"
    eval "$cmd"
    if [[ $? -ne 0 ]]; then
        error_exit "The creation of shoot namespace failed with exit code $?" "$?"
        return
    else
        echo "SHOOT_NAMESPACE: $SHOOT_NAMESPACE created successfully on virtual cluster"
    fi
  else
    echo "SHOOT_NAMESPACE $SHOOT_NAMESPACE already exists."
  fi

  echo "Getting available MachineClasses"
  cmdOut=$(kubectl get mcc -n "$SHOOT_NAMESPACE")
  if [[ -z "$cmdOut" ]]; then
    cmd="kubectl apply -f $mcc_path"
    echo "Applying MachineClasses using: ${cmd}"
    eval "$cmd"
    if [[ $? -ne 0 ]]; then
        error_exit "The creation of MachineClasses failed with exit code $?" "$?"
        return
    else
        echo "MachinClasses at $mcc_path created successfully on virtual cluster"
    fi
  else
    echo "MachineClasses already appear deployed in SHOOT_NAMESPACE $SHOOT_NAMESPACE. Skipping application"
  fi

  echo "Getting available MachineDeployments"
  cmdOut=$(kubectl get mcd -n "$SHOOT_NAMESPACE")
  if [[ -z "$cmdOut" ]]; then
    cmd="kubectl apply -f $mcd_path"
    echo "Applying MachineDeployments using: ${cmd}"
    eval "$cmd"
    if [[ $? -ne 0 ]]; then
        error_exit "The creation of MachineDeployments  failed with exit code $?" "$?"
        return
    else
        echo "MachineDeployments at $mcd_path created successfully on virtual cluster"
    fi
  else
    echo "MachineDeployments already appear deployed in SHOOT_NAMESPACE $SHOOT_NAMESPACE. Skipping application"
  fi

  echo "Getting available Secrets"
  cmdOut=$(kubectl get secret -n "$SHOOT_NAMESPACE")
  if [[ -z "$cmdOut" ]]; then
    cmd="kubectl apply -f $secret_dir"
    echo "Applying Secrets using: ${cmd}"
    eval "$cmd"
    if [[ $? -ne 0 ]]; then
        error_exit "The creation of Secrets  failed with exit code $?" "$?"
        return
    else
        echo "Secrets inside $secret_dir created successfully on virtual cluster"
    fi
  else
    echo "Secrets already appear deployed in SHOOT_NAMESPACE $SHOOT_NAMESPACE. Skipping application"
  fi

  echo "init_local_cluster completed."
}
