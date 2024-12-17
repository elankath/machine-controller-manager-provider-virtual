package virtual

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/elankath/machine-controller-manager-provider-virtual/virtual/awsfake"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/driver"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machinecodes/codes"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machinecodes/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"maps"
	"slices"
	"time"
)

const (
	// ProviderAWS string const to identify AWS provider
	ProviderAWS                  = "AWS"
	resourceTypeInstance         = "instance"
	resourceTypeVolume           = "volume"
	resourceTypeNetworkInterface = "network-interface"
	// awsEBSDriverName is the name of the CSI driver for EBS
	awsEBSDriverName = "ebs.csi.aws.com"
	awsPlacement     = "machine.sapcloud.io/awsPlacement"
)

var _ driver.Driver = &DriverImpl{}

// DriverImpl is the struct that implements the MCM driver.Driver interface
type DriverImpl struct {
	client       *kubernetes.Clientset
	managedNodes map[string]corev1.Node
}

func NewDriver(kubeconfig string) (driver.Driver, error) {
	// Create a config based on the kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create rest.Config from kubeconfig %q: %w", kubeconfig, err)
	}
	// Create a Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("cannot create clientset from kubeconfig %q: %w", kubeconfig, err)
	}
	return &DriverImpl{client: clientset, managedNodes: make(map[string]corev1.Node)}, nil
}

func (d *DriverImpl) CreateMachine(ctx context.Context, req *driver.CreateMachineRequest) (resp *driver.CreateMachineResponse, err error) {
	// Check if the MachineClass is for the supported cloud provider
	if req.MachineClass.Provider != ProviderAWS {
		err = fmt.Errorf("requested for Provider '%s', virtual provider currently only supports '%s'", req.MachineClass.Provider, ProviderAWS)
		err = status.Error(codes.InvalidArgument, err.Error())
		return
	}
	node, err := newNode(req.Machine, req.MachineClass)
	if err != nil {
		err = status.Error(codes.Internal, err.Error())
		return
	}
	instanceID, err := generateEC2InstanceID()
	if err != nil {
		err = status.Error(codes.Internal, err.Error())
		return
	}
	node.Spec.ProviderID = awsfake.EncodeInstanceID(req.MachineClass.NodeTemplate.Region, instanceID)
	_, err = d.client.CoreV1().Nodes().Create(ctx, &node, metav1.CreateOptions{})
	if err != nil {
		err = status.Error(codes.Internal, err.Error())
		return
	}
	adjustedNode, err := adjustNode(d.client, node.Name)
	if err != nil {
		err = status.Error(codes.Internal, err.Error())
		return
	}
	resp = &driver.CreateMachineResponse{
		ProviderID:     node.Spec.ProviderID,
		NodeName:       node.Name,
		LastKnownState: fmt.Sprintf("Instance %q created at %q", node.Name, time.Now()),
	}
	d.managedNodes[node.Name] = adjustedNode
	return
}

func adjustNode(client *kubernetes.Clientset, nodeName string) (adjustedNode corev1.Node, err error) {
	node, err := client.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("adjustNode cannot get node with name %q: %w", node.Name, err)
		return
	}
	node.Spec.Taints = slices.DeleteFunc(node.Spec.Taints, func(taint corev1.Taint) bool {
		return taint.Key == "node.kubernetes.io/not-ready"
	})
	//nd.Spec.Taints = lo.Filter(nd.Spec.Taints, func(item corev1.Taint, index int) bool {
	//	return item.Key != "node.kubernetes.io/not-ready"
	//})
	nd, err := client.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("adjustNode cannot update node with name %q: %w", nd.Name, err)
		return
	}
	nd.Status.Phase = corev1.NodeRunning
	nd, err = client.CoreV1().Nodes().UpdateStatus(context.Background(), nd, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("adjustNode cannot update the status of node with name %q: %w", nd.Name, err)
	}
	adjustedNode = *nd.DeepCopy()
	return
}

func newNode(machine *v1alpha1.Machine, machineClass *v1alpha1.MachineClass) (node corev1.Node, err error) {
	//providerSpec, err := awsfake.DecodeProviderSpecAndSecret(machineClass)
	//if err != nil {
	//	return
	//}
	nodeName := machine.Name // not really accurate with AWS but easier
	node.ObjectMeta = metav1.ObjectMeta{
		Name:   nodeName,
		Labels: map[string]string{},
	}
	node.Status = corev1.NodeStatus{
		Capacity: maps.Clone(machineClass.NodeTemplate.Capacity),
	}
	node.Status.Capacity[corev1.ResourcePods] = resource.MustParse("110")                    // very sucky
	node.Status.Capacity["nvidia.com/gpu"] = machineClass.NodeTemplate.Capacity["gpu"]       // TODO: weird - does not come in node.status.capacity nor allocatable
	node.Status.Capacity[corev1.ResourceEphemeralStorage] = resource.MustParse("50225972Ki") // hard-coding
	node.Status.Capacity["hugepages-1Gi"] = *resource.NewQuantity(0, resource.DecimalSI)
	node.Status.Capacity["hugepages-2Mi"] = *resource.NewQuantity(0, resource.DecimalSI)

	node.Status.Allocatable = maps.Clone(node.Status.Capacity)
	node.Status.Allocatable[corev1.ResourceEphemeralStorage] = resource.MustParse("48859825524") // hard-coding
	mem := node.Status.Capacity[corev1.ResourceMemory].DeepCopy()

	// subtracting 1.65 GB which includes 1Gb for kube reserved.
	subMem, err := resource.ParseQuantity("1.65Gi")
	if err != nil {
		return
	}
	allocatableMem := &mem
	allocatableMem.Sub(subMem)
	node.Status.Allocatable[corev1.ResourceMemory] = *allocatableMem

	if len(node.Annotations) == 0 {
		node.Annotations = make(map[string]string)
	}
	node.Annotations["volumes.kubernetes.io/controller-managed-attach-detach"] = "true"
	node.Labels[corev1.LabelArchStable] = *machineClass.NodeTemplate.Architecture
	node.Labels[corev1.LabelHostname] = nodeName
	node.Labels[corev1.LabelOSStable] = "linux"
	node.Labels[corev1.LabelInstanceType] = machineClass.NodeTemplate.InstanceType
	node.Labels["node.gardener.cloud/machine-name"] = machine.Name
	node.Labels["networking.gardener.cloud/node-local-dns-enabled"] = "true"

	for k, v := range machine.Spec.NodeTemplateSpec.Labels {
		node.Labels[k] = v
	}
	node.Status.Conditions = BuildReadyConditions()

	return

}

func (d *DriverImpl) InitializeMachine(ctx context.Context, request *driver.InitializeMachineRequest) (*driver.InitializeMachineResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Virtual Provider does not yet implement InitializeMachine")
}

func (d *DriverImpl) DeleteMachine(ctx context.Context, request *driver.DeleteMachineRequest) (response *driver.DeleteMachineResponse, err error) {
	delete(d.managedNodes, request.Machine.Name)
	return
}

func (d *DriverImpl) GetMachineStatus(ctx context.Context, request *driver.GetMachineStatusRequest) (response *driver.GetMachineStatusResponse, err error) {
	// TODO: introduce simulation of failures here.
	node, ok := d.managedNodes[request.Machine.Name]
	if !ok {
		err = status.Error(codes.NotFound, fmt.Sprintf("instance %q not found", request.Machine.Name))
		return
	}
	response = &driver.GetMachineStatusResponse{
		NodeName:   node.Name,
		ProviderID: node.Spec.ProviderID,
	}
	return
}

func (d *DriverImpl) ListMachines(ctx context.Context, request *driver.ListMachinesRequest) (response *driver.ListMachinesResponse, err error) {
	response = &driver.ListMachinesResponse{
		MachineList: make(map[string]string),
	}
	for _, node := range d.managedNodes {
		response.MachineList[node.Spec.ProviderID] = node.Name
	}
	return
}

func (d *DriverImpl) GetVolumeIDs(ctx context.Context, request *driver.GetVolumeIDsRequest) (response *driver.GetVolumeIDsResponse, err error) {
	// TODO: implemen in future to simulate attachment/detachment logic
	return
}

func BuildReadyConditions() []corev1.NodeCondition {
	lastTransition := time.Now().Add(-time.Minute)
	return []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{Time: lastTransition},
		},
		{
			Type:               corev1.NodeNetworkUnavailable,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Time{Time: lastTransition},
		},
		{
			Type:               corev1.NodeDiskPressure,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Time{Time: lastTransition},
		},
		{
			Type:               corev1.NodeMemoryPressure,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Time{Time: lastTransition},
		},
	}
}

func generateEC2InstanceID() (instanceID string, err error) {
	// EC2 instance IDs start with "i-" followed by a 17-character hexadecimal string
	const instanceIDPrefix = "i-"
	const idLength = 17 // Length of the hexadecimal string

	// Generate 17 random bytes
	randomBytes := make([]byte, idLength/2+1) // Ensure enough bytes for hex conversion
	_, err = rand.Read(randomBytes)
	if err != nil {
		return
	}

	// Convert random bytes to a hex string and truncate to 17 characters
	randomHex := fmt.Sprintf("%x", randomBytes)[:idLength]

	// Concatenate the prefix and the hex string
	instanceID = instanceIDPrefix + randomHex
	return
}
