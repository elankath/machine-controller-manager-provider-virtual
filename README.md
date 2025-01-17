# machine-controller-manager-provider-virtual

A virtual provider for the Gardener Machine Controller Manager thar provides a [Driver](https://github.com/gardener/machine-controller-manager/blob/f73366907e5c7a6c7b6fe2dad846ad6b646986db/pkg/util/provider/driver/driver.go#L17) implementation that creates virtual k8s `Nodes` in a virtual shoot cluster. It can mimic AWS/GCP/Azure `Nodes` depending on the `MachineClass`. At the moment only AWS is supported. A helper CLI tool `dev` is also provided to setup and then start services: KVCL (virtual cluster: `api-server`, `etcd`, `kube-scheduler`), MCM (`machine-controller-manager`), CA (`cluster-autoscaler`) as well as MC (this virtual `machine-controller`)

## Purpose

To enable dev-testing and debugging of the MCM and the CA on your local box with low resource usage and very simple setup. No Docker Desktop, no Kind, no complexity - just plain OS processes who write their logs to the `/tmp` directory.

## Usage

### Checkout Pre-requisite projects

1. KVCL (kubernetes virtual cluster). Checkout https://github.com/unmarshall/kvcl/ into your GOPATH. Ex: into `$GOPATH/src/github.com/unmarshall/kvcl`
1. Autoscaler (gardener autoscaler). Checkout https://github.com/gardener/autoscaler/ into `$GOPATH/src/k8s.io/autoscaler`.
1. MCM (machine-controller-manager). Checkout https://github.com/gardener/machine-controller-manager/ into `$GOPATH/src/github.com/elankath/machine-controller-manager`

### Setup

> [!NOTE]
> Make sure you are signed into the SAP network before executing setup!

#### Build the dev tool

1. Change to the project base directory
1. Execute: `go build -v -o bin/dev cmd/dev/main.go`

#### Execute Dev Setup
1. Execute: `./bin/dev setup -h` to view command help
1. Execute: `./bin/dev setup -project <gardenerProjName> -shoot <gardenerShootName>`


##### Dev Setup Help
```shell
➤ ./bin/dev setup -h                                                                                                                 git:main*
Usage of setup:
  -ca-dir string
    	CA Project Dir - fallback to env CA_DIR (default "/Users/I034796/go/src/k8s.io/autoscaler/cluster-autoscaler")
  -kvcl-dir string
    	KVCL Project Dir - fallback to env KVCL_DIR (default "/Users/I034796/go/src/github.com/unmarshall/kvcl")
  -landscape string
    	SAP Gardener Landscape - fallback to env LANDSCAPE (default "sap-landscape-dev")
  -mcm-dir string
    	MCM Project Dir - fallback to env MCM_DIR (default "/Users/I034796/go/src/github.com/gardener/machine-controller-manager")
  -project string
    	Gardener Project - fallback to env PROJECT
  -shoot string
    	Gardener Shoot Name - fallback to env SHOOT
  -skip-build
    	Skips building binaries if already present
```


#### What does 'dev setup' do ?
1. Will download/build the binaries for the virtual cluster, MCM, MC, CA  etc
1. It also downloads `MachineClass`, `MachineDeployment`, `Secrets` of the machine class and other resources from a real world Gardener cluster specified by the `-lanscape`, `-project` and `-shoot` options.
1. The idea is to set up things in such a way that the MCM, MC and CA components can use the configuration of a remote gardener cluster replicated on a local virtual cluster.
1. NOTE: GENERATES `StartConfig` inside `gen/start-config.json`. 
   1. KINDLY EDIT this file to customize local startup options of gardener MCM (machine-controller-manager), MC (virtual machine-controller) and CA (cluster-autoscaler)

### Dev Start

1. Execute: `./bin/dev start -h` to view command help
 
##### Dev Start Help

```shell
Usage of start:
  -all
    	Starts ALL services
  -ca
    	Start CA (gardener cluster-autoscaler)
  -mc
    	Start MC (virtual machine-controller)
  -mcm
    	Start MCM (gardener machine-controller-manager)

NOTE: "start" with no specified option starts ONLY KVCL (virtual-cluster)
```

#### Launch All Services

1. Execute: `./bin/dev start -all` #launches KVCL, MCM, MC and CA

#### Launch Only KVCL
1. Execute: `./bin/dev start` 

#### Launch KVCL and MCM
1. Execute: `./bin/dev start -mcm` 

#### Launch KVCL, MCM and MC
1. Execute: `./bin/dev start -mcm -mc`

#### Launch KVCL, MCM,  MC and CA 
1. Execute: `./bin/dev start -mcm -mc -ca`


### Dev Status
1. Execute: `./bin/dev status -h` to view command help

##### Dev Status Help

```shell
Usage of status:
  -all
    	check status of ALL services
  -ca
    	check status CA (gardener cluster-autoscaler)
  -mc
    	check status MC (virtual machine-controller)
  -mcm
    	check status MCM (gardener machine-controller-manager)

NOTE: "status" with no  option specified checks status of only kvcl (virtual-cluster)
```

### Examples

#### Checking out cluster resources

1. source `gen/env` # Sources generated env variables such as `KUBECONFIG` and `SHOOT_NAMESPACE`
1. Listing control plane objects
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
