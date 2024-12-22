#!/usr/bin/env zsh

script_dir=$(dirname "${(%):-%x}")
source "$script_dir/common.sh"

validate_grab_resources() {
  echo "NOTE: grab_resources requires KUBECONFIG, LANDSCAPE(default:sap-LANDSCAPE-dev), PROJECT, SHOOT env-variables to be set."
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

  validate_grab_resources

  local cmd="gardenctl target --garden $LANDSCAPE --project $PROJECT --shoot $SHOOT --control-plane"
  echo "$cmd"
  eval "$cmd"
  eval $(gctl kubectl-env zsh)
  kubectl get mcc -oyaml > "$mcc_path"
  kubectl get mcd -oyaml > "$mcd_path"
  local shoot_secrets=($(kubectl get secrets -o custom-columns=NAME:.metadata.name | grep '^shoot--' | tail +1))
  for i in {1..${#shoot_secrets[@]}}; do
    echo "Downloading secret ${shoot_secrets[i]} into ${dir}"
    kubectl get secret "${shoot_secrets[i]}" -oyaml > "${secret_dir}/${shoot_secrets[i]}.yaml"
  done
  #kubectl get secret cloudprovider -oyaml > "${init_secret_dir}/cloudprovider.yaml"
  kubectl get secret cloudprovider -oyaml > "${secret_dir}/cloudprovider.yaml"
  shootNs=$(kubectl config view --minify -o jsonpath='{.contexts[0].context.namespace}')
  echo "export SHOOT_NAMESPACE=$shootNs" > "$env_path"
  echo "export PROJECT=$PROJECT" >> "$env_path"
  echo "export SHOOT=$SHOOT" >> "$env_path"
  echo "export KUBECONFIG=$local_kubeconfig" >> "$env_path"

  echo "Downloading shoot worker yaml into $worker_spec_path ..."
  kubectl get worker "$SHOOT" -oyaml > "$worker_spec_path"

  echo "Downloading CA deployment yaml into $ca_deploy_spec_path ..."
  kubectl get deployment cluster-autoscaler -oyaml > "$ca_deploy_spec_path"

  echo "NOTE: Resources downloaded into $data_dir."
  echo "NOTE: Env variables are set inside $data_dir/env."
  echo "grab_resources successfully completed!"

}
