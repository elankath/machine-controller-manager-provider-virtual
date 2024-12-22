#!/usr/bin/env zsh
set -eo pipefail

script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/grab.sh"

# FIXME: Refactor setup-old.sh to use the variables inside common.sh for binDir etc. Move more code into common.sh

mkdir -p "bin/remote"

declare mode=local

if ! command -v yq &> /dev/null; then
    warn "yq not present. Installing..."
    brew install yq
    error_exit "Please re-run this script in new terminal/shell instance" 1
fi

validate_grab_resources

if [[ "$mode" == "local" ]]; then
  goos=$(go env GOOS)
  goarch=$(go env GOARCH)
  binDir="$(realpath bin)"
else
  goos=linux
  goarch=amd64
  binDir="$(realpath bin)/$mode"
fi
echo "GOOS set to $goos, GOARCH set to $goarch"
echo "For build mode $mode, will build binaries into $binDir"


if [[ ! -f "$binDir/kube-apiserver" ]]; then
  printf "Installing setup-envtest...\n"
  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  envTestSetupCmd="setup-envtest --os $goos --arch $goarch use -p path"
  printf "Executing: %s\n" "$envTestSetupCmd"
  binaryAssetsDir=$(eval "$envTestSetupCmd")
  errorCode="$?"
  if [[ "$errorCode" -gt 0 ]]; then
        error_exit "EC: $errorCode. Error in executing $envTestSetupCmd. Exiting!" 2
  fi
  echo "setup-envtest downloaded binaries into $binaryAssetsDir"
  cp -fv "$binaryAssetsDir"/* "$binDir"
  echo "Copied binaries into $binDir"
else
  echo "kube-apiserver binary already present at $binDir/kube-apiserver. Skipping setup-envtest install."
fi


if [[ ! -f "$kvcl_bin_path" ]]; then
  check_set_kvcl_dir
  pushd "$KVCL_DIR" > /dev/null
  echo "Building KVCL..."
  GOOS=$goos GOARCH=$goarch go build -v  -buildvcs=true  -o "$kvcl_bin_path" cmd/main.go
  chmod +x "$kvcl_bin_path"
  echo "KVCL Binary Built at $kvcl_bin_path"
else
  echo "KVCL Binary already present at $kvcl_bin_path. Skipping build."
fi


if [[ ! -f "$binDir/cluster-autoscaler" ]]; then
  check_set_ca_dir
  pushd "$CA_DIR" > /dev/null
  echo "Building CA..."
  GOOS=$goos GOARCH=$goarch go build -v  -buildvcs=true  -o "$binDir/cluster-autoscaler" main.go
  chmod +x "$binDir/cluster-autoscaler"
  echo "CA Binary Built at $ca_bin_path"
else
  echo "CA Binary already present at $binDir/cluster-autoscaler. Skipping build."
fi

check_set_mcm_dir

if [[ ! -f "$binDir/machine-controller-manager" ]]; then
  pushd "$MCM_DIR" > /dev/null
  echo "Building MCM..."
  GOOS=$goos GOARCH=$goarch go build -v -buildvcs=true  -o "$mcm_bin_path"  cmd/machine-controller-manager/controller_manager.go
  echo "MCM Binary Built at $mcm_bin_path"
  chmod +x "$mcm_bin_path"
else
  echo "MCM Binary already present at $binDir/machine-controller-manager. Skipping build."
fi

if [[ ! -f $mc_bin_path ]]; then
  cd "$project_dir"
  echo "Building MC (machine-controller-manager-provider-virtual)..."
  GOOS=$goos GOARCH=$goarch go build -v -buildvcs=true  -o "$mc_bin_path"  cmd/machine-controller/main.go
  echo "MC (virtual) Binary Built at $mc_bin_path"
  chmod +x "$mc_bin_path"
else
  echo "MC Binary already present at $mc_bin_path. Skipping build."
fi

if [[ ! -f "$hack_bin_path" ]]; then
  echo "Building Hack..."
  cd "$project_dir"
  go build -v -buildvcs=true  -o "$hack_bin_path"  cmd/dev/main.go
  echo "Hack Binary Built at $hack_bin_path"
else
  echo "Hack Binary already present at $hack_bin_path Skipping build."
fi

grab_resources

export KUBECONFIG="$local_kubeconfig"

echo "Setup Complete! You can now use ./hack/launch.sh"