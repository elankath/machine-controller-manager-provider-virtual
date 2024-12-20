#!/usr/bin/env zsh

script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/init.sh"

set -eo pipefail
verify() {
  echo "NOTE: You can edit $ca_deploy_spec_path before launching this script"
  check_ca_deploy_spec

  check_kvcl_running
  check_ca_binary
  check_hack_binary

}

deploy_dummy_mcm() {
  local cmdOut
  dummy_mcm_spec=$(cat <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: machine-controller-manager
  namespace: $SHOOT_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: machine-controller-manager
  template:
    metadata:
      labels:
        app: machine-controller-manager
    spec:
      containers:
        - name: dummy-container
          image: busybox:1.35.0-uclibc
          command: ["sleep", "infinity"]
EOF
)

  local dummy_mcm_spec_path="$gen_tmp_dir/mcm-dummy.yaml"
  echo "$dummy_mcm_spec" > "$dummy_mcm_spec_path"

#  if ! kubectl get deployment machine-controller-manager -n "$SHOOT_NAMESPACE" >/dev/null 2>&1;  then
#    cmd="kubectl apply -f $dummy_mcm_spec_path"
#    echo "Applying Dummy MCM using: ${cmd}"
#    eval "$cmd"
#    if [[ $? -ne 0 ]]; then
#        error_exit "The creation of dummy MCM failed with exit code $?" "$?"
#        return
#    else
#        echo "Dummy MCM created successfully on virtual-cluster"
#    fi
#  else
#    echo "Dummy MCM appears already deployed on virtual cluster"
#  fi
#  echo "Patching dummy MCM availableReplicas to 1."

  # This is Needed due to check in CA that checks for mcm available replicas
  eval "$hack_bin_path" -mcd-replicas=1
 # kubectl scale  deployment -n "$SHOOT_NAMESPACE" machine-controller-manager --replicas=1
}

main() {
  verify
  init_local_cluster


  ca_args=$(yq '.spec.template.spec.containers[0].command' $ca_deploy_spec_path | sed 's/^- //; 1d; /--kubeconfig/d; /--v/d;' | tr '\n' ' ')
  #  relative_path=${absolute_path#$base_path/}
#  local ca_bin_relative_path=${ca_bin_path#$project_dir}
#  echo "CA relative_path is $ca_bin_relative_path"

  source "$env_path"
  export KUBECONFIG="$local_kubeconfig"
  export CONTROL_KUBECONFIG="$KUBECONFIG"
  export CONTROL_NAMESPACE="$SHOOT_NAMESPACE"
  export TARGET_KUBECONFIG="$KUBECONFIG"

  deploy_dummy_mcm
  ca_cmd="bin/cluster-autoscaler $ca_args --v=4 --kubeconfig=$KUBECONFIG --leader-elect=false 2>&1 | tee $ca_log_file"
  echo "$ca_cmd"
  eval $ca_cmd
#  $ca_cmd 2>&1 | tee "$ca_log_file" &
#  echo "Return code is $?"
#  local ca_pid="$!"
#  echo "$ca_pid" > "$ca_pid_path"
#  sleep 6
#  echo "NOTE: CA LAUNCHED with PID $ca_pid written to $ca_pid_path. Logs output to $ca_log_file. CONTROL/TARGET KUBECONFIG=$KUBECONFIG"
#  tail -f /dev/null #sleep forever

}

# Run main function
main "$@"
