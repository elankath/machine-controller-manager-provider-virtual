# machine-controller-manager-provider-virtual

A virtual provider for the Gardener Machine Controller Manager thar provides a [Driver](https://github.com/gardener/machine-controller-manager/blob/f73366907e5c7a6c7b6fe2dad846ad6b646986db/pkg/util/provider/driver/driver.go#L17) implementation that creates virtual k8s `Nodes` in a virtual shoot cluster. It can mimic AWS/GCP/Azure `Nodes` depending on the `MachineClass`. At the moment only AWS is supported.

## Usage

### Setup 
Execute `./hack/setup.sh`

- This will setup download/build the binaries for the virtual cluster, MCM and other control plane components.
- You need to export `LANDSCAPE`, `PROJECT` and `SHOOT` env variables before running so that the `MachineClasses` and `MachineDeployments` can be copied from an existing gardener cluster.
- It downloads relevant files into `gen` sub-dir of project. A `env` file with env variables are also generated at `gen/env`.

### Launch
Execute `./hack/launch.sh`

- This script will launch the virtual cluster and `machine-controller-manager`

TODO: describe working in detail

### CLI Use

1. `export KUBECONFIG=/tmp/kvcl.yaml`
2. Listing control plane objects
   1. `kubectl config set-context --current --namespace=<SHOOT_NAMESPACE>`
   2. `kubecrtl get mc`
3. Listing data plane objects
   1. `kubectl get no`

## Design

TODO