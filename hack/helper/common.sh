# NOTE: This is a bash module not meant to be directly invoked but only sourced into dependent scripts.
declare script_helper_dir=$(dirname "${(%):-%x}")
declare project_dir="$(realpath ${script_helper_dir}/../../)"
declare bin_dir="${project_dir}/bin"
declare gen_dir="${project_dir}/gen"

declare hack_bin_path="$bin_dir/hack"
declare kvcl_bin_path="$bin_dir/kvcl"
declare mcm_bin_path="$bin_dir/machine-controller-manager"
declare mc_bin_path="$bin_dir/machine-controller"
declare ca_bin_path="$bin_dir/cluster-autoscaler"

declare kvcl_log_file="/tmp/kvcl.log"
declare mcm_log_file="/tmp/mcm.log"
declare mc_log_file="/tmp/mc.log"
declare ca_log_file="/tmp/ca.log"

declare data_dir="${project_dir}/gen/data"
[[ -d "$data_dir" ]] || mkdir -p "$data_dir"

declare gen_tmp_dir="${project_dir}/gen/tmp"
[[ -d "$gen_tmp_dir" ]] || mkdir -p "$gen_tmp_dir"

declare ca_launch_path="${gen_dir}/ca-launch.sh"
declare mcm_launch_path="${gen_dir}/mcm-launch.sh"

declare mcc_path="$data_dir/mcc.yaml"
declare mcd_path="$data_dir/mcd.yaml"
declare secret_dir="$data_dir/scrt"
[[ -d "$secret_dir" ]] || mkdir -p "$secret_dir"

declare env_path="$data_dir/env"
declare worker_spec_path="${gen_tmp_dir}/worker.yaml"
declare ca_deploy_spec_path="${gen_tmp_dir}/ca-deploy.yaml"

declare kvcl_pid_path="/tmp/kvcl.pid"
declare ca_pid_path="/tmp/ca.pid"
declare mcm_pid_path="/tmp/mcm.pid"
declare mc_pid_path="/tmp/mc.pid"
declare local_kubeconfig="/tmp/kvcl.yaml"
export KUBECONFIG="$local_kubeconfig" # TODO: Allow parameterization here later.

error_exit() {
    echo "Error: $1" >&2
    if [[ -z "$ZSH_SUBSCRATCH" ]]; then
        exit "${2:-1}"
    fi
}

warn() {
    echo "Warning: $1" >&2
}

log_error() {
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "$timestamp ERROR: $1" >> "$LOG_FILE"
    echo "ERROR: $1" >&2
}
check_set_kvcl_dir() {
  if [[ -z "$KVCL_DIR" ]]; then
    export KVCL_DIR="$GOPATH/src/github.com/unmarshall/kvcl"
    echo "KVCL_DIR is not set. Assuming default: $KVCL_DIR"
  fi
  exists_dir_or_exit "$KVCL_DIR" "KVCL_DIR: $KVCL_DIR doesn't exist. Kindly check out at this path or explicitly export KVCL_DIR before invoking this script" 4
}

check_set_mcm_dir() {
  if [[ -z "$MCM_DIR" ]]; then
    export MCM_DIR="$GOPATH/src/github.com/gardener/machine-controller-manager"
    echo "MCM_DIR is not set. Assuming default: $MCM_DIR"
  fi
  exists_dir_or_exit "$MCM_DIR" "MCM_DIR: $MCM_DIR doesn't exist. Kindly check out at this path or explicitly export MCM_DIR before invoking this script" 4
  export MCM_CRD_DIR="$MCM_DIR/kubernetes/crds"
  exists_dir_or_exit "$MCM_CRD_DIR" "$MCM_CRD_DIR doesn't exist. Kindly ensure that MCM is checked out correctly before invoking this script" 4
}

check_set_ca_dir() {
  if [[ -z "$CA_DIR" ]]; then
    export CA_DIR="$GOPATH/src/k8s.io/autoscaler/cluster-autoscaler"
    echo "CA_DIR (cluster-autoscaler directory) is not set. Assuming default: $CA_DIR"
  fi
  exists_dir_or_exit "$CA_DIR" "CA_DIR: $CA_DIR doesn't exist. Kindly check out at this path or explicitly export CA_DIR before invoking this script" 4
}

debug() {
    if [ "$VERBOSE" = true ]; then
        echo "DEBUG: $1" >&2
    fi
}

exists_dir_or_exit() {
  if [[ ! -d "$1" ]]; then
    error_exit "$2" "$3"
  fi
  echo "$1 exists"
}

exists_file_or_exit() {
  if [[ ! -f "$1" ]]; then
    error_exit "$2" "$3"
  fi
  echo "$1 exists"
}

kill_by_name() {
  proc_name=$1
  if [[ -z "$proc_name" ]]; then
    error_exit "kill_by_name: process name is empty"
  fi
  local pids=$(pgrep -f "$proc_name")
  if [[ ! -z "$pids" ]]; then
#    echo "Found pids $pids for $proc_name"
    for p in $pids; do
#      echo "Killing process $p ..."
      kill -9 "$p"
    done
  fi
}
check_ca_deploy_spec() {
  if [[ ! -f "$ca_deploy_spec_path" ]]; then
    error_exit "check_ca_deploy_spec: No CA Deployment Spec at $ca_deploy_spec_path.\nKindly first run ./hack/setup.sh"
  fi
}

check_kvcl_running() {
  if [[ ! -f "$kvcl_pid_path" ]]; then
    error_exit "check_kvcl_running: No kvcl PID at $kvcl_pid_path. Kindly first run ./hack/launch-kvcl.sh"
  fi
}

check_hack_binary() {
  echo "Hack Binary should be at: $hack_bin_path"
  if [[ ! -f "$hack_bin_path" ]]; then
    error_exit "check_hack_binary: No hack binary at: $hack_bin_path. Kindly first run ./hack/setup.sh"
  fi
}
check_ca_binary() {
  echo "CA Binary should be at: $ca_bin_path"
  if [[ ! -f "$ca_bin_path" ]]; then
    error_exit "check_ca_binary: No CA binary at: $ca_bin_path. Kindly first run ./hack/setup.sh"
  fi
}

check_mcm_binary() {
  echo "MCM Binary should be at: $mcm_bin_path"
  if [[ ! -f "$mcm_bin_path" ]]; then
    error_exit "check_mcm_binary: No MCM binary at: $mcm_bin_path. Kindly first run ./hack/setup.sh"
  fi
}

check_mc_binary() {
  echo "MC Binary should be at: $mc_bin_path"
  if [[ ! -f "$mc_bin_path" ]]; then
    error_exit "check_mc_binary: No MCM Binary at: $mc_bin_path. Kindly first run ./hack/setup.sh"
  fi
}
