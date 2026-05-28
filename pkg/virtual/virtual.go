package virtual

import (
	"cmp"
	"context"
	crand "crypto/rand" // ← preferred alias
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	rand "math/rand/v2"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/elankath/machine-controller-manager-provider-virtual/pkg/virtual/awsfake"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	machineclientset "github.com/gardener/machine-controller-manager/pkg/client/clientset/versioned"
	machineclientbuilder "github.com/gardener/machine-controller-manager/pkg/util/clientbuilder/machine"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/driver"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machinecodes/codes"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machinecodes/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	// ProviderAWS string const to identify AWS provider
	ProviderAWS         = "AWS"
	QuotaPrefixFmt      = "QUOTA_%d"
	QuotaMachineTypeFmt = QuotaPrefixFmt + "_MACHINE_TYPE"
	QuotaRegionFmt      = QuotaPrefixFmt + "_REGION"
	QuotaAmountFmt      = QuotaPrefixFmt + "_AMOUNT"
)

var SimulationConfigPath = "gen/simulation-config.json"
var _ driver.Driver = &DriverImpl{}

// DriverImpl is the struct that implements the MCM driver.Driver interface
type DriverImpl struct {
	mu                  sync.Mutex
	clientConfig        *rest.Config
	client              *kubernetes.Clientset
	machineClient       machineclientset.Interface
	shootNamespace      string
	managedNodes        map[string]corev1.Node
	simConfig           SimulationConfig
	lastSimConfigChange time.Time
}

type QuotaLookup struct {
	MachineType string
	RegionName  string
}

type SimulationConfig struct {
	Quotas         []Quota
	InstanceDelays InstanceDelays
}

// InstanceDelays represents the minimum and maximum delays in seconds taken to create, initialize or join instance to cluster.
// The real value will be randomized between minimum and maximum
type InstanceDelays struct {
	CreateMin     int64
	CreateMax     int64
	InitializeMin int64
	InitializeMax int64
	JoinMin       int64
	JoinMax       int64
	DeleteMin     int64
	DeleteMax     int64
}

type Quota struct {
	MachineType string
	Region      string
	Amount      int
}

func (q Quota) String() string {
	return fmt.Sprintf("(Region:%s, MachineType:%s, Amount:%d", q.Region, q.MachineType, q.Amount)
}

func NewDriver(ctx context.Context, kubeconfig string, shootNamespace string) (driver.Driver, error) {
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
	mcb := machineclientbuilder.SimpleClientBuilder{
		ClientConfig: config,
	}
	machineClient, err := mcb.Client("machine-controller")
	if err != nil {
		return nil, fmt.Errorf("cannot create machine client: %w", err)
	}
	d := &DriverImpl{clientConfig: config,
		client:         clientset,
		machineClient:  machineClient,
		shootNamespace: shootNamespace,
		managedNodes:   make(map[string]corev1.Node)}
	err = d.reloadNodes(ctx)
	if err != nil {
		return nil, err
	}
	err = d.createSimulationConfig(ctx)
	if err != nil {
		return nil, err
	}
	go d.watchSimulationConfig()
	return d, nil
}

func (d *DriverImpl) reloadNodes(ctx context.Context) error {
	nodeList, err := d.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	d.managedNodes = make(map[string]corev1.Node)
	for _, n := range nodeList.Items {
		d.managedNodes[n.Name] = n
	}
	return nil
}

func (d *DriverImpl) createSimulationConfig(ctx context.Context) error {
	if FileExists(SimulationConfigPath) {
		klog.Infof("Simlulation Config already exists at %q - loading", SimulationConfigPath)
		return d.refreshSimulationConfig()
	}

	machineIf := d.machineClient.MachineV1alpha1()
	machineClassList, err := machineIf.MachineClasses(d.shootNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	var nt *v1alpha1.NodeTemplate
	//var quotaKeys = sets.New[string]()
	mccItems := machineClassList.Items
	slices.SortFunc(mccItems, func(a, b v1alpha1.MachineClass) int {
		return cmp.Compare(a.Name, b.Name)
	})
	var quotas []Quota
	for i := 0; i < len(mccItems); i++ {
		nt = mccItems[i].NodeTemplate
		quotas = append(quotas, Quota{
			//MachineTypeKey: machineypeKey,
			MachineType: nt.InstanceType,
			//RegionKey:      regionKey,
			Region: nt.Region,
			//AmountKey:      amountKey,
			Amount: 10,
		})
	}
	d.simConfig.Quotas = quotas
	d.simConfig.InstanceDelays = InstanceDelays{
		CreateMin:     1,
		CreateMax:     2,
		InitializeMin: 1,
		InitializeMax: 2,
		JoinMin:       2,
		JoinMax:       4,
		DeleteMin:     1,
		DeleteMax:     2,
	}
	data, err := json.MarshalIndent(d.simConfig, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(SimulationConfigPath, data, 0755)
	if err != nil {
		return err
	}
	d.lastSimConfigChange = time.Now().UTC()
	klog.Infof("createSimulationConfig wrote SimulationConfig at %q", SimulationConfigPath)
	return nil
}

func hasSimulationConfigChanged(markTime time.Time) bool {
	fileInfo, err := os.Stat(SimulationConfigPath)
	if err != nil {
		klog.Errorf("cannot fetch file info for %q: %v", SimulationConfigPath, err)
		return false
	}
	lastModifiedTime := fileInfo.ModTime().UTC()
	return lastModifiedTime.After(markTime)
}

func (d *DriverImpl) watchSimulationConfig() {
	for {
		select {
		case <-time.After(time.Second * 10):
			if hasSimulationConfigChanged(d.lastSimConfigChange) {
				err := d.refreshSimulationConfig()
				if err != nil {
					klog.Errorf("watchSimulationConfig cannot refreshSimulationConfig: %w", err)
				}
			}
		}
	}
}

func (d *DriverImpl) refreshSimulationConfig() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := os.ReadFile(SimulationConfigPath)
	if err != nil {
		return err
	}
	var sm SimulationConfig
	err = json.Unmarshal(data, &sm)
	if err != nil {
		return err
	}
	d.simConfig = sm
	d.lastSimConfigChange = time.Now().UTC()
	klog.Infof("refreshSimulationConfig reloaded %q at %q, simConfig=%v", SimulationConfigPath, d.lastSimConfigChange, d.simConfig)
	return nil
}

func (d *DriverImpl) countNodesForRegionAndMachineType(region, machineType string) (count int) {
	for _, n := range d.managedNodes {
		if n.Labels[corev1.LabelTopologyRegion] == region && n.Labels[corev1.LabelInstanceTypeStable] == machineType {
			count++
		}
	}
	return
}

func (d *DriverImpl) CreateMachine(ctx context.Context, req *driver.CreateMachineRequest) (resp *driver.CreateMachineResponse, err error) {
	klog.Infof("Driver.CreateMachine started.")
	d.mu.Lock()
	defer d.mu.Unlock()
	// Check if the MachineClass is for the supported cloud provider
	if req.MachineClass.Provider != ProviderAWS {
		err = fmt.Errorf("requested for Provider '%s', virtual provider currently only supports '%s'", req.MachineClass.Provider, ProviderAWS)
		err = status.Error(codes.InvalidArgument, err.Error())
		return
	}
	var refQuota *Quota
	for _, q := range d.simConfig.Quotas {
		if req.MachineClass.NodeTemplate.Region == q.Region && req.MachineClass.NodeTemplate.InstanceType == q.MachineType {
			refQuota = &q
		}
	}
	if refQuota != nil {
		num := d.countNodesForRegionAndMachineType(refQuota.Region, refQuota.MachineType)
		if num >= refQuota.Amount {
			msg := fmt.Sprintf("Quota %s exhausted", refQuota)
			klog.Error(msg)
			err = status.Error(codes.ResourceExhausted, msg)
			return
		}
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
	node.Status.Conditions = BuildReadyConditions(corev1.ConditionFalse)
	node.Status.Phase = corev1.NodePending
	delay := randomDuration(d.simConfig.InstanceDelays.CreateMin, d.simConfig.InstanceDelays.CreateMax)
	klog.Infof("Simulating a delay in creation of %s for %q", delay, req.Machine.Name)
	<-time.After(delay)
	_, err = d.client.CoreV1().Nodes().Create(ctx, &node, metav1.CreateOptions{})
	if err != nil {
		err = status.Error(codes.Internal, err.Error())
		return
	}
	klog.Infof("Created NotReady node %q", node.Name)
	resp = &driver.CreateMachineResponse{
		ProviderID:     node.Spec.ProviderID,
		NodeName:       node.Name,
		LastKnownState: fmt.Sprintf("Instance %q created at %q", node.Name, time.Now()),
	}

	joinDelay := randomDuration(d.simConfig.InstanceDelays.JoinMin, d.simConfig.InstanceDelays.JoinMax)
	go func() {
		klog.Infof("Waiting for joinDelay %q before making node %q Ready", joinDelay, node.Name)
		<-time.After(joinDelay)
		_, err = makeNodeReady(d.client, node.Name)
		if err != nil {
			klog.Errorf("Failed to make node %q Ready: %v", node.Name, err)
		}
		err = d.reloadNodes(ctx)
		if err != nil {
			klog.Error("Failed to reload nodes", err)
		}
		d.changeAssignedPodsToRunning(ctx) //TODO: Introduce SimulationConfig.PodReadyMinDelay, PodReadyMaxDelay
	}()
	err = d.reloadNodes(ctx)
	klog.Infof("Driver.CreateMachine ended.")
	return
}

// randomDuration returns a random duration between min  and max (inclusive).
func randomDuration(minSecs, maxSecs int64) time.Duration {
	minNano := time.Second.Nanoseconds() * minSecs
	maxNano := time.Second.Nanoseconds() * maxSecs
	return time.Duration(rand.Int64N(maxNano) + minNano)
}

func makeNodeReady(client *kubernetes.Clientset, nodeName string) (adjustedNode corev1.Node, err error) {
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
		err = fmt.Errorf("makeNodeReady cannot update node with name %q: %w", nd.Name, err)
		return
	}
	nd.Status.Phase = corev1.NodeRunning
	nd.Status.Conditions = BuildReadyConditions(corev1.ConditionTrue)
	nd, err = client.CoreV1().Nodes().UpdateStatus(context.Background(), nd, metav1.UpdateOptions{})
	if err != nil {
		err = fmt.Errorf("makeNodeReady cannot update the status of node with name %q: %w", nd.Name, err)
	}
	adjustedNode = *nd.DeepCopy()
	klog.Infof("makeNodeReady made node %q Ready", nd.Name)
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
	node.Labels[corev1.LabelInstanceTypeStable] = machineClass.NodeTemplate.InstanceType
	node.Labels["node.gardener.cloud/machine-name"] = machine.Name
	node.Labels["networking.gardener.cloud/node-local-dns-enabled"] = "true"
	node.Labels[corev1.LabelTopologyRegion] = machineClass.NodeTemplate.Region
	node.Labels[corev1.LabelInstanceType] = machineClass.NodeTemplate.InstanceType

	for k, v := range machine.Spec.NodeTemplateSpec.Labels {
		node.Labels[k] = v
	}
	return

}

func (d *DriverImpl) InitializeMachine(ctx context.Context, request *driver.InitializeMachineRequest) (*driver.InitializeMachineResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Virtual Provider does not yet implement InitializeMachine")
}

func (d *DriverImpl) DeleteMachine(ctx context.Context, request *driver.DeleteMachineRequest) (response *driver.DeleteMachineResponse, err error) {
	d.mu.Lock()
	delay := randomDuration(d.simConfig.InstanceDelays.DeleteMin, d.simConfig.InstanceDelays.DeleteMax)
	klog.Infof("Simulating a delay in deletion of %s for %q", delay, request.Machine.Name)
	defer func() {
		<-time.After(delay)
	}()
	defer d.mu.Unlock()
	delete(d.managedNodes, request.Machine.Name)
	return
}

func (d *DriverImpl) GetMachineStatus(ctx context.Context, request *driver.GetMachineStatusRequest) (response *driver.GetMachineStatusResponse, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
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
	go d.changeAssignedPodsToRunning(ctx)
	return
}

func (d *DriverImpl) ListMachines(ctx context.Context, request *driver.ListMachinesRequest) (response *driver.ListMachinesResponse, err error) {
	response = &driver.ListMachinesResponse{
		MachineList: make(map[string]string),
	}
	for _, node := range d.managedNodes {
		response.MachineList[node.Spec.ProviderID] = node.Name
	}
	go d.changeAssignedPodsToRunning(ctx)
	return
}

func (d *DriverImpl) GetVolumeIDs(ctx context.Context, request *driver.GetVolumeIDsRequest) (response *driver.GetVolumeIDsResponse, err error) {
	// TODO: implemen in future to simulate attachment/detachment logic
	return
}

func (d *DriverImpl) changeAssignedPodsToRunning(ctx context.Context) {
	pods, err := d.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("changeAssignedPodsToRunning failed to list pods due to %v", err)
	}
	// Iterate over the pods and patch those in "Pending" status
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodPending && pod.Spec.NodeName != "" {
			// Example patch: add a label to trigger changes (as a simple trigger)
			//podStatus := pod.Status.DeepCopy()
			//podStatus.Phase = corev1.PodRunning
			pod.Status.Phase = corev1.PodRunning
			_, err = d.client.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, &pod, metav1.UpdateOptions{})
			if err != nil {
				klog.Infof("changeAssignedPodsToRunning FAILED to change  pod %q assigned to %q to phase Running: %s", pod.Name, pod.Spec.NodeName, err)
			} else {
				klog.Infof("changeAssignedPodsToRunning changed pod %q assigned to %q to phase Running", pod.Name, pod.Spec.NodeName)
			}
		}
	}
}

func BuildReadyConditions(readyConditionStatus corev1.ConditionStatus) []corev1.NodeCondition {
	heartBeatTime := metav1.NewTime(time.Now())
	return []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             readyConditionStatus,
			LastTransitionTime: heartBeatTime,
			LastHeartbeatTime:  heartBeatTime,
		},
		{
			Type:               corev1.NodeNetworkUnavailable,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: heartBeatTime,
			LastHeartbeatTime:  heartBeatTime,
		},
		{
			Type:               corev1.NodeDiskPressure,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: heartBeatTime,
			LastHeartbeatTime:  heartBeatTime,
		},
		{
			Type:               corev1.NodeMemoryPressure,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: heartBeatTime,
			LastHeartbeatTime:  heartBeatTime,
		},
	}
}

func generateEC2InstanceID() (string, error) {
	const prefix = "i-"
	const hexLength = 17

	// We need 9 bytes to get at least 17 hex characters
	bytes := make([]byte, 9)

	if _, err := crand.Read(bytes); err != nil {
		return "", err
	}

	hexStr := hex.EncodeToString(bytes)

	// Truncate to exactly 17 characters
	if len(hexStr) > hexLength {
		hexStr = hexStr[:hexLength]
	}

	return prefix + hexStr, nil
}

func FileExists(filepath string) bool {
	fileinfo, err := os.Stat(filepath)
	if err != nil {
		return false
	}
	if fileinfo.IsDir() {
		return false
	}
	return true
}
