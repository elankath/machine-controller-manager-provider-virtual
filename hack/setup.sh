#!/usr/bin/env zsh
set -eo pipefail

script_dir=$(dirname "${(%):-%x}")
source "$script_dir/helper/common.sh"
source "$script_dir/helper/grab.sh"
proj_dir=$(realpath "$script_dir/..")

mkdir -p "bin/remote"

declare mode=local

if ! command -v direnv &> /dev/null; then
    warn "direnv not present. Installing..."
    brew install direnv
    error_exit "Kindly exit your current terminal and relaunch this script in new terminal/shell instance" 1
    exit 1
fi

declare envrc_file="$proj_dir/.envrc"
touch "$envrc_file"
direnv allow "$proj_dir"

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


printf "Installing setup-envtest...\n"
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
envTestSetupCmd="setup-envtest --os $goos --arch $goarch use -p path"
printf "Executing: %s\n" "$envTestSetupCmd"
binaryAssetsDir=$(eval "$envTestSetupCmd")
echo "export BINARY_ASSETS_DIR=$binaryAssetsDir" >> "$envrc_file"
errorCode="$?"
if [[ "$errorCode" -gt 0 ]]; then
      error_exit "EC: $errorCode. Error in executing $envTestSetupCmd. Exiting!" 2
fi
echo "setup-envtest downloaded binaries into $binaryAssetsDir"
cp -fv "$binaryAssetsDir"/* "$binDir"
echo "Copied binaries into $binDir"

if [[ -z "$KVCL_DIR" ]]; then
  KVCL_DIR="$GOPATH/src/github.com/unmarshall/kvcl"
  echo "KVCL_DIR is not set. Assuming default: $KVCL_DIR"
fi

exists_dir_or_exit "$KVCL_DIR" "KVCL_DIR: $KVCL_DIR doesn't exist. Kindly check out at this path or explicitly set KVCL_DIR before invoking this script" 3
pushd "$KVCL_DIR" > /dev/null
echo "Building KVCL..."
GOOS=$goos GOARCH=$goarch go build -v  -buildvcs=true  -o "$binDir/kvcl" cmd/main.go
chmod +x "$binDir/kvcl"

check_set_mcm_dir

pushd "$MCM_DIR" > /dev/null
echo "Building MCM..."
GOOS=$goos GOARCH=$goarch go build -v -buildvcs=true  -o "$binDir/machine-controller-manager"  cmd/machine-controller-manager/controller_manager.go

grab_resources

echo "Setup Complete! You can now use ./hack/launch.sh"