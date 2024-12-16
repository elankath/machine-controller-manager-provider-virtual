declare init_data_dir="/tmp/mcmpv"
[[ -d "$init_data_dir" ]] || mkdir -p "$init_data_dir"

declare init_mcc_path="$init_data_dir/mcc.yaml"
declare init_mcd_path="$init_data_dir/mcd.yaml"
declare init_env_path="$init_data_dir/env"

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

check_set_mcm_dir() {
  if [[ -z "$MCM_DIR" ]]; then
    export MCM_DIR="$GOPATH/src/github.com/gardener/machine-controller-manager"
    echo "MCM_DIR is not set. Assuming default: $MCM_DIR"
  fi
  exists_dir_or_exit "$MCM_DIR" "MCM_DIR: $MCM_DIR doesn't exist. Kindly check out at this path or explicitly export MCM_DIR before invoking this script" 4
  export MCM_CRD_DIR="$MCM_DIR/kubernetes/crds"
  exists_dir_or_exit "$MCM_CRD_DIR" "$MCM_CRD_DIR doesn't exist. Kindly ensure that MCM is checked out correctly before invoking this script" 4
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
