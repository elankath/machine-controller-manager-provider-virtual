package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	pu "github.com/elankath/machine-controller-manager-provider-virtual/hack/projectutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"os"
	"os/exec"
	"path"
	"strings"
)

const (
	ExitBasicInvocation = iota + 1
	ExitOptionUnspecified
	ExitInvalidGoModuleDir
	ExitInstallControlPlane
	ExitBuildKVCL
	ExitBuildMCMCABinaries
	ExitDownloadClusterData
	ExitSerDeser
)

const ProjBinDir = "bin"
const ProjGenDir = "gen"

var DownloadSpecDir = path.Join(ProjGenDir, "spec")

// Download holds paths of downloadable cluster resources
var Download = struct {
	MachineClassesSpecPath     string
	MachineDeploymentsSpecPath string
	WorkerSpecPath             string
	CADeploySpecPath           string
	MCMDeploySpecPath          string
	CAPriorityExpanderSpecPath string
	SecretSpecsDir             string
	ClusterInfoPath            string
	ScriptEnvPath              string
}{
	MachineClassesSpecPath:     path.Join(DownloadSpecDir, "mcc.yaml"),
	MachineDeploymentsSpecPath: path.Join(DownloadSpecDir, "mcd.yaml"),
	WorkerSpecPath:             path.Join(DownloadSpecDir, "worker.yaml"),
	CADeploySpecPath:           path.Join(DownloadSpecDir, "cluster-autoscaler.yaml"),
	MCMDeploySpecPath:          path.Join(DownloadSpecDir, "machine-controller-manager.yaml"),
	CAPriorityExpanderSpecPath: path.Join(DownloadSpecDir, "cluster-autoscaler-priority-expander.yaml"),
	SecretSpecsDir:             path.Join(DownloadSpecDir, "scrt"),
	ClusterInfoPath:            path.Join(ProjGenDir, "cluster-info.json"),
	ScriptEnvPath:              path.Join(ProjGenDir, "env"),
}

func main() {
	var exitCode int
	var err error

	if !pu.Exists("virtual/virtual.go") {
		_, _ = fmt.Fprintln(os.Stderr, "hack: Please invoke hack tool from project base directory")
		os.Exit(ExitBasicInvocation)
	}
	if len(os.Args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "hack: Expected one of 'hack setup [opts]' or 'hack start [opts]'")
		os.Exit(ExitBasicInvocation)
	}

	var ctx = context.Background()
	var command = os.Args[1]
	switch command {
	case "setup":
		exitCode, err = Setup(ctx)
	case "start":
		exitCode, err = Start(ctx)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "hack: error: Unknown subcommand %q\n", command)
		os.Exit(ExitBasicInvocation)
	}
	if exitCode == 0 {
		klog.Infof("hack %s SUCCESSFUL", command)
		return
	} else {
		klog.Errorf("hack %s FAILED: %v", command, err)
		os.Exit(exitCode)
	}
}

type SetupOpts struct {
	pu.ClusterCoordinate
	KVCLDir   string
	MCMDir    string
	CADir     string
	SkipBuild bool
	//Mode      string
}

func Setup(ctx context.Context) (exitCode int, err error) {
	var so SetupOpts
	setupCmd := flag.NewFlagSet("setup", flag.ExitOnError)
	defaultLandscape := os.Getenv("LANDSCAPE")
	if defaultLandscape == "" {
		defaultLandscape = "sap-landscape-dev"
	}
	defaultKVCLDir := os.Getenv("KVCL_DIR")
	if defaultKVCLDir == "" {
		//defaultKVCLDir = filepath.Join(pu.GoPathDir, "src/github.com/unmarshall/kvcl")
		defaultKVCLDir = pu.GetGoSourceDir("github.com/unmarshall/kvcl")
	}
	defaultMCMDir := os.Getenv("MCM_DIR")
	if defaultMCMDir == "" {
		//defaultMCMDir = filepath.Join(pu.GoPathDir, "src/github.com/gardener/machine-controller-manager")
		defaultMCMDir = pu.GetGoSourceDir("github.com/gardener/machine-controller-manager")
	}
	defaultCADir := os.Getenv("CA_DIR")
	if defaultCADir == "" {
		//defaultCADir = filepath.Join(pu.GoPathDir, "src/k8s.io/autoscaler/cluster-autoscaler")
		defaultCADir = pu.GetGoSourceDir("k8s.io/autoscaler/cluster-autoscaler")
	}
	setupCmd.StringVar(&so.Landscape, "landscape", defaultLandscape, "SAP Gardener Landscape - fallback to env LANDSCAPE")
	setupCmd.StringVar(&so.Project, "project", os.Getenv("PROJECT"), "Gardener Project - fallback to env PROJECT")
	setupCmd.StringVar(&so.Shoot, "shoot", os.Getenv("SHOOT"), "Gardener Shoot Name - fallback to env SHOOT")
	setupCmd.StringVar(&so.KVCLDir, "kvcl-dir", defaultKVCLDir, "KVCL Project Dir - fallback to env KVCL_DIR")
	setupCmd.StringVar(&so.MCMDir, "mcm-dir", defaultMCMDir, "MCM Project Dir - fallback to env MCM_DIR")
	setupCmd.StringVar(&so.CADir, "ca-dir", defaultCADir, "CA Project Dir - fallback to env CA_DIR")
	setupCmd.BoolVar(&so.SkipBuild, "skip-build", false, "Skips building binaries if already present")
	//setupCmd.StringVar(&so.Mode, "mode", "local", "Development Mode")
	err = setupCmd.Parse(os.Args[2:])
	if err != nil {
		exitCode = ExitBasicInvocation
		err = fmt.Errorf("error parsing flags: %w", err)
		return
	}
	exitCode, err = ValidateFlagsAreNotEmpty(map[string]string{
		"landscape": so.Landscape,
		"project":   so.Project,
		"shoot":     so.Shoot,
		"kvcl-dir":  so.KVCLDir,
		"mcm-dir":   so.MCMDir,
		"ca-dir":    so.CADir,
	})
	if err != nil {
		return
	}
	exitCode, err = ValidateProjectDirs(map[string]string{
		"kvcl-dir": so.KVCLDir,
		"mcm-dir":  so.MCMDir,
		"ca-dir":   so.CADir,
	})
	if err != nil {
		return
	}

	exitCode, err = InstallControlPlane(ctx)
	if err != nil {
		return
	}

	err = pu.GoBuild(ctx, so.KVCLDir, "cmd/main.go", path.Join(ProjBinDir, "kvcl"), so.SkipBuild)
	if err != nil {
		exitCode = ExitBuildKVCL
		err = fmt.Errorf("error building KVCL (k8s virtual cluster): %w", err)
		return
	}

	err = BuildMCMCABinaries(ctx, so)
	if err != nil {
		exitCode = ExitBuildMCMCABinaries
		return
	}
	err = DownloadClusterData(ctx, so.ClusterCoordinate)
	if err != nil {
		exitCode = ExitDownloadClusterData
		return
	}
	return
}

func DownloadClusterData(ctx context.Context, coord pu.ClusterCoordinate) (err error) {
	var clusterInfo pu.ClusterInfo
	clusterInfo, ok := ReadClusterInfo()
	if ok {
		if clusterInfo.ClusterCoordinate != coord {
			klog.Warningf("DownloadClusterData deleting all downloaded files detected change in cluster coordinate from %v->%v",
				clusterInfo.ClusterCoordinate, coord)
			err = os.RemoveAll(ProjGenDir)
			if err != nil {
				return err
			}
			err = os.MkdirAll(ProjGenDir, 0755)
			if err != nil {
				return err
			}
		}
	}
	gctl := pu.NewGardenCtl(coord)

	err = pu.CreateIfNotExists(DownloadSpecDir, 0755)
	if err != nil {
		return
	}
	if !pu.Exists(Download.MachineClassesSpecPath) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, fmt.Sprintf("kubectl get mcc -oyaml > %s", Download.MachineClassesSpecPath))
		if err != nil {
			return
		}
		klog.Infof("Downloaded MachineClasses YAML into %q", Download.MachineClassesSpecPath)
	} else {
		klog.Infof("MachineClasses YAML already present at %q", Download.MachineClassesSpecPath)
	}

	if !pu.Exists(Download.MachineDeploymentsSpecPath) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, fmt.Sprintf("kubectl get mcd -oyaml > %s", Download.MachineDeploymentsSpecPath))
		if err != nil {
			return
		}
		klog.Infof("Downloaded MachineDeployments YAML into %q - skipping download.", Download.MachineDeploymentsSpecPath)
	} else {
		klog.Infof("MachineDeployments YAML already present at %q - skipping download.", Download.MachineDeploymentsSpecPath)
	}

	if !pu.Exists(Download.WorkerSpecPath) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, fmt.Sprintf("kubectl get worker -oyaml > %s", Download.WorkerSpecPath))
		if err != nil {
			return
		}
		klog.Infof("Downloaded Worker YAML into %q - skipping download.", Download.WorkerSpecPath)
	} else {
		klog.Infof("Worker YAML already present at %q - skipping download.", Download.WorkerSpecPath)
	}

	if !pu.Exists(Download.CADeploySpecPath) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, fmt.Sprintf("kubectl get deploy cluster-autoscaler -oyaml > %s", Download.CADeploySpecPath))
		if err != nil {
			return
		}
		klog.Infof("Downloaded CA Deploy YAML into %q.", Download.CADeploySpecPath)
	} else {
		klog.Infof("CA Deploy YAML already present at %q - skipping download.", Download.CADeploySpecPath)
	}
	if !pu.Exists(Download.MCMDeploySpecPath) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, fmt.Sprintf("kubectl get deploy machine-controller-manager -oyaml > %s", Download.MCMDeploySpecPath))
		if err != nil {
			return
		}
		klog.Infof("Downloaded MCM Deploy YAML into %q.", Download.MCMDeploySpecPath)
	} else {
		klog.Infof("MCM Deploy YAML already present at %q - skipping download.", Download.MCMDeploySpecPath)
	}
	if !pu.Exists(Download.CAPriorityExpanderSpecPath) {
		var listCmOut string
		listCmOut, err = gctl.ExecuteCommandOnPlane(ctx, pu.DataPlane, "kubectl get cm -n kube-system")
		if err != nil {
			return
		}
		if strings.Contains(listCmOut, "cluster-autoscaler-priority-expande") {
			_, err = gctl.ExecuteCommandOnPlane(ctx, pu.DataPlane, fmt.Sprintf("kubectl get cm -n kube-system cluster-autoscaler-priority-expander -oyaml > %s", Download.CAPriorityExpanderSpecPath))
			if err != nil {
				return
			}
		}
		klog.Infof("Downloaded CA Priority Expander YAML into %q.", Download.MCMDeploySpecPath)
	} else {
		klog.Infof("CA Priority Expandder YAML already present at %q - skipping download.", Download.CAPriorityExpanderSpecPath)
	}

	listSecretsCmd := "kubectl get secrets -o custom-columns=NAME:.metadata.name | grep '^shoot--' | tail +1"
	listSecretsOut, err := gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, listSecretsCmd)
	if err != nil {
		return
	}
	secretNames := strings.Split(listSecretsOut, "\n")
	secretNames = append(secretNames, "cloudprovider")

	var sb strings.Builder
	for _, name := range secretNames {
		if strings.TrimSpace(name) == "" {
			continue
		}
		secretSpecPath := path.Join(Download.SecretSpecsDir, name+".yaml")
		if pu.FileExists(secretSpecPath) {
			klog.Infof("Secret already downloaded at %q - skipping download", secretSpecPath)
			continue
		}
		sb.WriteString("kubectl get secret ")
		sb.WriteString(name)
		sb.WriteString(" -oyaml > ")
		sb.WriteString(secretSpecPath)
		sb.WriteString(" ; ")
	}
	err = os.MkdirAll(Download.SecretSpecsDir, 0755)
	if err != nil {
		return
	}
	if sb.Len() > 0 {
		downloadSecretsCmd := sb.String()
		_, err = gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, downloadSecretsCmd)
		if err != nil {
			return
		}
	}
	shootNamespace, err := gctl.ExecuteCommandOnPlane(ctx, pu.ControlPlane, "kubectl config view --minify -o jsonpath='{.contexts[0].context.namespace}'")
	klog.Infof("shootNamespace: %q", shootNamespace)
	clusterInfo = pu.ClusterInfo{
		ClusterCoordinate: coord,
		ShootNamespace:    shootNamespace,
	}
	data, err := json.Marshal(clusterInfo)
	if err != nil {
		return
	}
	err = os.WriteFile(Download.ClusterInfoPath, data, 0755)
	if err != nil {
		return
	}
	sb.Reset()
	sb.WriteString("export LANDSCAPE=")
	sb.WriteString(clusterInfo.Landscape)
	sb.WriteString("\n")
	sb.WriteString("export PROJECT=")
	sb.WriteString(clusterInfo.Project)
	sb.WriteString("\n")
	sb.WriteString("export SHOOT=")
	sb.WriteString(clusterInfo.Shoot)
	sb.WriteString("\n")
	sb.WriteString("export SHOOT_NAMESPACE=")
	sb.WriteString(shootNamespace)
	sb.WriteString("\n")
	err = os.WriteFile(Download.ScriptEnvPath, []byte(sb.String()), 0755)
	if err != nil {
		return
	}
	return
}

func ReadClusterInfo() (clusterInfo pu.ClusterInfo, ok bool) {
	ok = false
	p := Download.ClusterInfoPath
	if !pu.FileExists(p) {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		klog.Errorf("ReadClusterInfo failed to read %q: %v", p, err)
		return
	}
	err = json.Unmarshal(data, &clusterInfo)
	if err != nil {
		klog.Errorf("ReadClusterInfo failed to un-marshall %q: %v", p, err)
		return
	}
	ok = true
	return
}

func BuildMCMCABinaries(ctx context.Context, so SetupOpts) (err error) {
	err = pu.GoBuild(ctx, so.CADir, "main.go", path.Join(ProjBinDir, "cluster-autoscaler"), so.SkipBuild)
	if err != nil {
		err = fmt.Errorf("error building CA (cluster-autoscaler): %w", err)
		return
	}
	err = pu.GoBuild(ctx, so.MCMDir, "cmd/machine-controller-manager/controller_manager.go", path.Join(ProjBinDir, "machine-controller-manager"), so.SkipBuild)
	if err != nil {
		err = fmt.Errorf("error building MCM (machine-controller-manager): %w", err)
		return
	}
	err = pu.GoBuild(ctx, ".", "cmd/machine-controller/main.go", path.Join(ProjBinDir, "machine-controller"), so.SkipBuild)
	if err != nil {
		err = fmt.Errorf("error building this project - virtual MC (machine-controller): %w", err)
		return
	}
	return
}

func InstallControlPlane(ctx context.Context) (exitCode int, err error) {
	asBinPath := path.Join(ProjBinDir, "kube-apiserver")
	if pu.FileExists(asBinPath) {
		klog.Infof("InstallControlPlane: %s exists. Assuming control plane binaries are already downloaded by setup-envtest.", asBinPath)
		return
	}
	err = pu.GoInstall(ctx, "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest")
	if err != nil {
		exitCode = ExitInstallControlPlane
		err = fmt.Errorf("error installing setup-envtest: %v", err)
		return
	}

	kubeBinAssetsPath, err := InvokeSetupEnvTest(ctx)
	if err != nil {
		exitCode = ExitInstallControlPlane
		err = fmt.Errorf("error installing kube binary assets using setup-envtest: %v", err)
		return
	}
	klog.Infof("Kube Binary Assets Path: %s", kubeBinAssetsPath)
	err = pu.CopyAllFiles(kubeBinAssetsPath, ProjBinDir)
	if err != nil {
		exitCode = ExitInstallControlPlane
		err = fmt.Errorf("error copying kube binary assets: %w", err)
		return
	}
	return 0, nil
}

type LaunchOpts struct {
}

func Start(ctx context.Context) (exitCode int, err error) {
	//var lo LaunchOpts
	//startCmd := flag.NewFlagSet("Start", flag.ExitOnError)
	//startCmd.StringVar(&lo.Kubeconfig, "kubeconfig", "/tmp/kvcl.yaml", "Kubeconfig file path")
	//startCmd.StringVar(&lo.ShootNamespace, "shoot-ns", os.Getenv("SHOOT_NAMESPACE"), "Shoot Namespace")
	//startCmd.IntVar(&lo.MCDReplicas, "mcd-replicas", 1, "Patch machine-controller-manager replicas and availableReplicas to given value")

	return
}

//func LaunchOld(ctx context.Context) (exitCode int, err error) {
//	var lo LaunchOpts
//	launchCmd := flag.NewFlagSet("Start", flag.ExitOnError)
//	launchCmd.StringVar(&lo.Kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Kubeconfig file path")
//	launchCmd.StringVar(&lo.ShootNamespace, "shoot-ns", os.Getenv("SHOOT_NAMESPACE"), "Shoot Namespace")
//	launchCmd.IntVar(&lo.MCDReplicas, "mcd-replicas", 1, "Patch machine-controller-manager replicas and availableReplicas to given value")
//	err = launchCmd.Parse(os.Args[2:])
//	if err != nil {
//		exitCode = ExitBasicInvocation
//		err = fmt.Errorf("error parsing flags: %w", err)
//		return
//	}
//	if lo.Kubeconfig == "" {
//		return 1, fmt.Errorf("-kubeconfig flag is required")
//	}
//	if lo.ShootNamespace == "" {
//		return 1, fmt.Errorf("-shoot-name flag is required")
//	}
//	// Create a config based on the kubeconfig file
//	config, err := clientcmd.BuildConfigFromFlags("", lo.Kubeconfig)
//	if err != nil {
//		return 2, fmt.Errorf("cannot create rest.Config from kubeconfig %q: %w", lo.Kubeconfig, err)
//	}
//	// Create a Kubernetes clientset
//	clientset, err := kubernetes.NewForConfig(config)
//	if err != nil {
//		return 2, fmt.Errorf("cannot create clientset from kubeconfig %q: %w", lo.Kubeconfig, err)
//	}
//	if lo.MCDReplicas > 0 {
//		err = createUpdateDummyMCD(ctx, clientset, lo.ShootNamespace, lo.MCDReplicas)
//		if err != nil {
//			return 3, err
//		}
//	}
//	return 0, nil
//}

func createUpdateDummyMCD(ctx context.Context, client kubernetes.Interface, shootNamespace string, numReplicas int) error {
	var deployment *appsv1.Deployment
	deploymentClient := client.AppsV1().Deployments(shootNamespace)
	deployment, err := deploymentClient.Get(ctx, "machine-controller-manager", metav1.GetOptions{})
	ptrReplicas := ptr.To[int32](int32(numReplicas))
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Dummy machine-controller manager deployment does not exist, Creating with replicas %d", numReplicas)
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine-controller-manager",
					Namespace: shootNamespace,
					Labels: map[string]string{
						"app":                 "kubernetes",
						"gardener.cloud/role": "controlplane",
						"role":                "machine-controller-manager",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptrReplicas, // Set the number of replicas
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "kubernetes",
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":                 "kubernetes",
								"gardener.cloud/role": "controlplane",
								"role":                "machine-controller-manager",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "machine-controller-manager",
									Image: "your-image:tag", // Replace with actual image
								},
							},
						},
					},
				},
			}
			deployment, err = deploymentClient.Create(ctx, deployment, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("cannot create dummy machine-controller-manager deployment: %w", err)
			}
		} else {
			return fmt.Errorf("cannot get dummy machine-controller-manager: %w", err)
		}
	}
	deployment.Spec.Replicas = ptrReplicas
	deployment.Status.Replicas = int32(numReplicas)
	deployment.Status.ReadyReplicas = int32(numReplicas)
	deployment.Status.AvailableReplicas = int32(numReplicas)
	deployment, err = deploymentClient.UpdateStatus(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("dev cannot update replicas to %q for dummy machine-controller-manager deployment: %w", numReplicas, err)
	}
	klog.Infof("dev successfully updated replicas, availableReplicas of dummy machine-controller-manager to %d", numReplicas)
	return nil
}

func ValidateFlagsAreNotEmpty(flagNameVals map[string]string) (exitCode int, err error) {
	for k, v := range flagNameVals {
		if strings.TrimSpace(v) == "" {
			err = fmt.Errorf("flag '%s' is required but empty", k)
			exitCode = ExitOptionUnspecified
			return
		}
	}
	return
}

func ValidateProjectDirs(projDirs map[string]string) (exitCode int, err error) {
	for n, d := range projDirs {
		if !pu.DirExists(d) {
			err = fmt.Errorf("dir %q for option %q does not exist - kindly check out the project correctly at this path", n, d)
			exitCode = ExitInvalidGoModuleDir
		}
		if !pu.FileExists(path.Join(d, "go.mod")) {
			err = fmt.Errorf("%q for option %q is invalid - no go.mod - kindly check out the project correctly at this path", n, d)
			exitCode = ExitInvalidGoModuleDir
			return
		}
	}
	return
}

func InvokeSetupEnvTest(ctx context.Context) (kubeBinAssetsPath string, err error) {
	binPath := path.Join(pu.GoBinDir, "setup-envtest")
	cmd := exec.CommandContext(ctx, binPath, "use", "-p", "path")
	kubeBinAssetsPath, err = pu.InvokeCommand(cmd)
	if err != nil {
		return
	}
	return
}
