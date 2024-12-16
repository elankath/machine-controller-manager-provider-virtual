#!/usr/bin/env zsh

script_dir=$(dirname "${(%):-%x}")
source "$script_dir/common.sh"

grab_resources() {
  set -eo pipefail
  echo "grab_resources commencing..."
  echo "NOTE: grab_resources requires KUBECONFIG, LANDSCAPE(default:sap-LANDSCAPE-dev), PROJECT, SHOOT env-variables to be set."
#  if [[ -z "$KUBECONFIG" ]]; then
#    error_exit "Kindly set KUBECONFIG env before calling init_local_cluster" 10
#  fi


  if [[ -z "$LANDSCAPE" ]]; then
    LANDSCAPE="sap-landscape-dev"
  fi

  echo "Using LANDSCAPE: $LANDSCAPE"

  if [[ -z "$PROJECT" ]]; then
    error_exit "Kindly set name of project in PROJECT env before calling init_local_cluster" 10
  fi

  if [[ -z "$SHOOT" ]]; then
    error_exit "Kindly set name of shoot in SHOOT env before calling init_local_cluster" 10
  fi

  cmd="gardenctl target --garden $LANDSCAPE --project $PROJECT --shoot $SHOOT --control-plane"
  echo "$cmd"
  eval "$cmd"
  kubectl get mcc -oyaml > "$init_mcc_path"
  kubectl get mcd -oyaml > "$init_mcd_path"
  shootNs=$(kubectl config view --minify -o jsonpath='{.contexts[0].context.namespace}')
  echo "export SHOOT_NAMESPACE=$shootNs" > "$init_env_path"
  echo "export PROJECT=$PROJECT" >> "$init_env_path"
  echo "export SHOOT=$SHOOT" >> "$init_env_path"

#  check_set_mcm_dir
#  echo "Using KUBECONFIG: $KUBECONFIG"
#  export KUBECONFIG="$KUBECONFIG"
#
#  cmd="kubectl apply -f $MCM_CRD_DIR"
#  echo "Applying CRDs using: ${cmd}"
#  eval "$cmd"

echo "grab_resources completed. Resources downloaded into $init_data_dir. Environment downloaded into $init_data_dir/.envrc"

}
