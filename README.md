# machine-controller-manager-provider-virtual

A virtual provider for the Gardener Machine Controller Manager thar provides a [Driver](https://github.com/gardener/machine-controller-manager/blob/f73366907e5c7a6c7b6fe2dad846ad6b646986db/pkg/util/provider/driver/driver.go#L17) implementation that creates virtual k8s `Nodes` in a virtual shoot cluster. It can mimic AWS/GCP/Azure `Nodes` depending on the `MachineClass`. At the moment only AWS is supported.  Helper scripts are also provided to setup and then launch a virtual cluster, MCM (machine-controller-manager), CA (cluster-autoscaler) as well as MC (this virtual machine-controller)

## Usage

### Setup 

> [!NOTE]
> Make sure you are signed into the SAP network before executing setup!

Execute `./hack/setup.sh -project <gardenerProjName> -shoot <gardenerShootName>`

1. Will build the `hack` binary into `bin` and then invoke `bin/hack setup opts`
1. Will download/build the binaries for the virtual cluster, MCM, MC, CA, hack, etc
1. It also downloads `MachineClass`, `MachineDeployment`, `Secrets` of the machine class and other resources from a real world Gardener cluster specified by the `-lanscape`, `-project` and `-shoot` options.
1. The idea is to set up things in such a way that the MCM, MC and CA components can use the configuration of a remote gardener cluster replicated on a local virtual cluster.

### Launch

### Examples

#### Checking out resources
1. `export KUBECONFIG=/tmp/kvcl.yaml`
2. Listing control plane objects
   1. `kubectl config set-context --current --namespace=<SHOOT_NAMESPACE>`
   2. `kubectl get mcc,mcd,mc`
```shell
NAME                                                             AGE
machineclass.machine.sapcloud.io/shoot--i034796--aw-a-z1-ccb6a   11m
machineclass.machine.sapcloud.io/shoot--i034796--aw-b-z1-44b8a   11m
machineclass.machine.sapcloud.io/shoot--i034796--aw-c-z1-c7d6c   11m

NAME                                                            READY   DESIRED   UP-TO-DATE   AVAILABLE   AGE
machinedeployment.machine.sapcloud.io/shoot--i034796--aw-a-z1   1       1         1            1           11m
machinedeployment.machine.sapcloud.io/shoot--i034796--aw-b-z1                                              11m
machinedeployment.machine.sapcloud.io/shoot--i034796--aw-c-z1                                              11m

NAME                                                              STATUS    AGE   NODE
machine.machine.sapcloud.io/shoot--i034796--aw-a-z1-c9478-99wwq   Running   11m   shoot--i034796--aw-a-z1-c9478-99wwq
```
3. Listing data plane objects
   1. `kubectl get no`

## Design

TODO
