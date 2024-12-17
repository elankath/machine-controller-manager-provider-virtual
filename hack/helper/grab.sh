#!/usr/bin/env zsh

script_dir=$(dirname "${(%):-%x}")
source "$script_dir/common.sh"

validate_grab_resources() {
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
}
grab_resources() {
  set -eo pipefail
  echo "grab_resources commencing..."
  echo "NOTE: grab_resources requires KUBECONFIG, LANDSCAPE(default:sap-LANDSCAPE-dev), PROJECT, SHOOT env-variables to be set."
#  if [[ -z "$KUBECONFIG" ]]; then
#    error_exit "Kindly set KUBECONFIG env before calling init_local_cluster" 10
#  fi

  validate_grab_resources

  cmd="gardenctl target --garden $LANDSCAPE --project $PROJECT --shoot $SHOOT --control-plane"
  echo "$cmd"
  eval "$cmd"
  kubectl get mcc -oyaml > "$init_mcc_path"
  kubectl get mcd -oyaml > "$init_mcd_path"
  local shoot_secrets=($(kubectl get secrets -o custom-columns=NAME:.metadata.name | grep '^shoot--' | tail +1))
  for i in {1..${#shoot_secrets[@]}}; do
    echo "Downloading secret ${shoot_secrets[i]} into ${init_secret_dir}"
    kubectl get secret "${shoot_secrets[i]}" -oyaml > "${init_secret_dir}/${shoot_secrets[i]}.yaml"
  done
  kubectl get secret cloudprovider -oyaml > "${init_secret_dir}/cloudprovider.yaml"
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

echo "grab_resources completed. Resources downloaded into $init_data_dir. Environment variabes created within $init_data_dir/env"

}
