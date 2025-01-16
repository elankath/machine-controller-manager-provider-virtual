package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	du "github.com/elankath/machine-controller-manager-provider-virtual/pkg/devutil"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	ExitBasicInvocation = iota + 1
	ExitOptionUnspecified
	ExitInvalidGoModuleDir
	ExitInstallControlPlane
	ExitBuildKVCL
	ExitBuildMCMCABinaries
	ExitCopyCRDs
	ExitDownloadClusterData
	ExitGenerateStartConfig
	ExitLoadClusterInfo
	ExitLoadStartConfig
	ExitStartKVCL
	ExitCreateKubeClient
	ExitInitCluster
	ExitStartMCM
	ExitStartMC
	ExitStartCA
	ExitStopKVCL
	ExitStopCA
	ExitStopMC
	ExitStopMCM
	ExitStatusCheck
)

var (
	KVCLName = "kvcl"
	MCMName  = "machine-controller-manager"
	MCName   = "machine-controller"
	CAName   = "cluster-autoscaler"
)

type ProjectDirs struct {
	Base   string
	Gen    string
	Bin    string
	Spec   string
	Secret string
	CRD    string
}

type BinaryPaths struct {
	KVCL string
	MCM  string
	MC   string
	CA   string
}

type LogPaths struct {
	KVCL string
	MCM  string
	MC   string
	CA   string
}

type PidPaths struct {
	KVCL string
	MCM  string
	MC   string
	CA   string
}

type SpecPaths struct {
	MachineClasses     string
	MachineDeployments string
	Worker             string
	CADeploy           string
	MCMDeploy          string
	CAPriorityExpander string
}

type ConfigPaths struct {
	ClusterInfo     string
	LocalKubeConfig string
	EnvScript       string
	StartConfig     string
}

var Dirs ProjectDirs
var Bins BinaryPaths
var Specs SpecPaths
var Configs ConfigPaths
var Logs LogPaths
var Pids PidPaths

func init() {
	var err error

	if du.Exists("pkg/virtual/virtual.go") {
		Dirs.Base, err = filepath.Abs(".")
	} else if du.Exists("../../pkg/virtual/virtual.go") {
		klog.Infof("Executing in test mode")
		Dirs.Base, err = filepath.Abs("../../")
	} else {
		_, _ = fmt.Fprintln(os.Stderr, "dev: Please invoke dev tool from project base directory")
		os.Exit(ExitBasicInvocation)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "dev: Cannot resolve project base directory %q due to %v\n", Dirs.Base, err)
		os.Exit(ExitBasicInvocation)
	}
	Dirs.Gen = path.Join(Dirs.Base, "gen")
	Dirs.Bin = path.Join(Dirs.Base, "bin")
	Dirs.Spec = path.Join(Dirs.Gen, "spec")
	Dirs.Secret = path.Join(Dirs.Spec, "scrt")
	Dirs.CRD = path.Join(Dirs.Spec, "crd")
	//klog.Infof("ProjDirs: %v", Dirs)
	Bins = BinaryPaths{
		KVCL: path.Join(Dirs.Bin, KVCLName),
		MCM:  path.Join(Dirs.Bin, MCMName),
		MC:   path.Join(Dirs.Bin, MCName),
		CA:   path.Join(Dirs.Bin, CAName),
	}
	Specs = SpecPaths{
		MachineClasses:     path.Join(Dirs.Spec, "mcc.yaml"),
		MachineDeployments: path.Join(Dirs.Spec, "mcd.yaml"),
		Worker:             path.Join(Dirs.Spec, "worker.yaml"),
		CADeploy:           path.Join(Dirs.Spec, "cluster-autoscaler.yaml"),
		MCMDeploy:          path.Join(Dirs.Spec, "machine-controller-manager.yaml"),
		CAPriorityExpander: path.Join(Dirs.Spec, "cluster-autoscaler-priority-expander.yaml"),
	}
	Logs = LogPaths{
		KVCL: "/tmp/kvcl.log",
		MCM:  "/tmp/mcm.log",
		MC:   "/tmp/mc.log",
		CA:   "/tmp/ca.log",
	}
	Pids = PidPaths{
		KVCL: "/tmp/kvcl.pid",
		MCM:  "/tmp/mcm.pid",
		MC:   "/tmp/mc.pid",
		CA:   "/tmp/ca.pid",
	}
	Configs = ConfigPaths{
		ClusterInfo:     path.Join(Dirs.Gen, "cluster-info.json"),
		LocalKubeConfig: "/tmp/kvcl.yaml",
		EnvScript:       path.Join(Dirs.Gen, "env"),
		StartConfig:     path.Join(Dirs.Gen, "start-config.json"),
	}
}

func main() {
	var exitCode int
	var err error
	var cleanups []func()

	if len(os.Args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "dev: Expected one of 'dev setup [opts]' or 'dev start [opts]'")
		os.Exit(ExitBasicInvocation)
	}
	defer func() {
		for _, c := range cleanups {
			if c != nil {
				c()
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	command := os.Args[1]
	switch command {
	case "setup":
		exitCode, err = Setup(ctx)
	case "start":
		exitCode, err = Start(ctx, cancel)
		//if err == nil {
		//	du.WaitForSignalAndSdu.down(ctx, cancel)
		//}
	case "stop":
		exitCode, err = Stop(ctx)
	case "status":
		exitCode, err = Status(ctx)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "dev: error: Unknown subcommand %q\n", command)
		os.Exit(ExitBasicInvocation)
	}

	if exitCode > 0 {
		klog.Errorf("dev %s FAILED: %v", command, err)
		os.Exit(exitCode)
	}

	klog.Infof("dev %s DONE.", command)
	return
}

type SetupOpts struct {
	du.ClusterCoordinate
	KVCLDir   string
	MCMDir    string
	CADir     string
	SkipBuild bool
	// Mode      string
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
		// defaultKVCLDir = filepath.Join(du.GoPathDir, "src/github.com/unmarshall/kvcl")
		defaultKVCLDir = du.GetGoSourceDir("github.com/unmarshall/kvcl")
	}
	defaultMCMDir := os.Getenv("MCM_DIR")
	if defaultMCMDir == "" {
		// defaultMCMDir = filepath.Join(du.GoPathDir, "src/github.com/gardener/machine-controller-manager")
		defaultMCMDir = du.GetGoSourceDir("github.com/gardener/machine-controller-manager")
	}
	defaultCADir := os.Getenv("CA_DIR")
	if defaultCADir == "" {
		// defaultCADir = filepath.Join(du.GoPathDir, "src/k8s.io/autoscaler/cluster-autoscaler")
		defaultCADir = du.GetGoSourceDir("k8s.io/autoscaler/cluster-autoscaler")
	}
	setupCmd.StringVar(&so.Landscape, "landscape", defaultLandscape, "SAP Gardener Landscape - fallback to env LANDSCAPE")
	setupCmd.StringVar(&so.Project, "project", os.Getenv("PROJECT"), "Gardener Project - fallback to env PROJECT")
	setupCmd.StringVar(&so.Shoot, "shoot", os.Getenv("SHOOT"), "Gardener Shoot Name - fallback to env SHOOT")
	setupCmd.StringVar(&so.KVCLDir, "kvcl-dir", defaultKVCLDir, "KVCL Project Dir - fallback to env KVCL_DIR")
	setupCmd.StringVar(&so.MCMDir, "mcm-dir", defaultMCMDir, "MCM Project Dir - fallback to env MCM_DIR")
	setupCmd.StringVar(&so.CADir, "ca-dir", defaultCADir, "CA Project Dir - fallback to env CA_DIR")
	setupCmd.BoolVar(&so.SkipBuild, "skip-build", false, "Skips building binaries if already present")
	// setupCmd.StringVar(&so.Mode, "mode", "local", "Development Mode")
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

	err = du.GoBuild(ctx, so.KVCLDir, "cmd/main.go", Bins.KVCL, so.SkipBuild)
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
	err = CopyCRDs(so)
	if err != nil {
		exitCode = ExitCopyCRDs
		return
	}
	_, err = DownloadClusterData(ctx, so.ClusterCoordinate)
	if err != nil {
		exitCode = ExitDownloadClusterData
		return
	}

	err = GenerateStartConfig()
	if err != nil {
		exitCode = ExitGenerateStartConfig
		return
	}
	return
}

type DevOpts struct {
	MCM bool
	MC  bool
	CA  bool
	ALL bool
}

func Start(ctx context.Context, cancel context.CancelFunc) (exitCode int, err error) {
	var opts DevOpts
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	startCmd.BoolVar(&opts.MCM, "mcm", false, "Start MCM (gardener machine-controller-manager)")
	startCmd.BoolVar(&opts.MC, "mc", false, "Start MC (virtual machine-controller)")
	startCmd.BoolVar(&opts.CA, "ca", false, "Start CA (gardener cluster-autoscaler)")
	startCmd.BoolVar(&opts.ALL, "all", false, "Starts ALL services")

	standardUsage := startCmd.Usage
	startCmd.Usage = func() {
		w := flag.CommandLine.Output() // may be os.Stderr - but not necessarily
		standardUsage()
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "NOTE: %q with no specified option starts ONLY KVCL (virtual-cluster)", os.Args[1])
	}

	err = startCmd.Parse(os.Args[2:])
	if err != nil {
		exitCode = ExitBasicInvocation
		err = fmt.Errorf("error parsing flags: %w", err)
		return
	}
	clusterInfo, ok := ReadClusterInfo()
	if !ok {
		exitCode = ExitLoadClusterInfo
		err = fmt.Errorf("error loading ClusterInfo %q: %w", Configs.ClusterInfo, err)
		return
	}

	startConfig, err := du.ReadJson[StartConfig](Configs.StartConfig)
	if err != nil {
		exitCode = ExitLoadStartConfig
		err = fmt.Errorf("error loading StartConfig %q: %w", Configs.StartConfig, err)
		return
	}

	kvclRunning, kvclPid, err := du.CheckProcessRunning(Pids.KVCL)
	if err != nil {
		exitCode = ExitStopKVCL
		err = fmt.Errorf("error stopping KVCL: %w", err)
		return
	}

	if !kvclRunning {
		err = startKVCL(ctx, cancel)
		if err != nil {
			exitCode = ExitStartKVCL
			err = fmt.Errorf("error starting KVCL: %w", err)
			return
		}
	} else {
		klog.Infof("KVCL appears already launched with pid %d", kvclPid)
	}
	//cleanups = append(cleanups, cleanup)

	client, err := du.CreateKubeClient(ctx, Configs.LocalKubeConfig)
	if err != nil {
		exitCode = ExitCreateKubeClient
		return
	}

	if opts.MCM || opts.MC || opts.ALL {
		err = initVirtualCluster(ctx, client, clusterInfo.ShootNamespace)
		if err != nil {
			exitCode = ExitInitCluster
			err = fmt.Errorf("error initializing virtual cluster: %w", err)
			return
		}
	}

	var errs []error
	if opts.MCM || opts.ALL {
		err = startMCM(ctx, client, clusterInfo.ShootNamespace, startConfig)
		if err != nil {
			exitCode = ExitStartMCM
			err = fmt.Errorf("error starting MCM: %w", err)
			errs = append(errs, err)
		}
	}

	if opts.MC || opts.ALL {
		err = startMC(ctx, startConfig)
		if err != nil {
			exitCode = ExitStartMC
			err = fmt.Errorf("error starting MC: %w", err)
			errs = append(errs, err)
		}
	}

	if opts.CA || opts.ALL {
		err = startCA(ctx, client, clusterInfo.ShootNamespace, startConfig)
		if err != nil {
			exitCode = ExitStartCA
			err = fmt.Errorf("error starting CA: %w", err)
			errs = append(errs, err)
		}
	}
	err = errors.Join(errs...)
	return
}

func Stop(ctx context.Context) (exitCode int, err error) {
	var opts DevOpts
	stopCmd := flag.NewFlagSet("stop", flag.ExitOnError)
	standardUsage := stopCmd.Usage
	stopCmd.Usage = func() {
		w := flag.CommandLine.Output() // may be os.Stderr - but not necessarily
		standardUsage()
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "NOTE: %q with no specified option stops ALL services including kvcl (virtual-cluster)", os.Args[1])
	}
	stopCmd.BoolVar(&opts.CA, "ca", false, "Stop CA (gardener cluster-autoscaler)")
	stopCmd.BoolVar(&opts.MC, "mc", false, "Stop MC (virtual machine-controller)")
	stopCmd.BoolVar(&opts.MCM, "mcm", false, "Stop MCM (gardener machine-controller-manager)")
	stopCmd.BoolVar(&opts.ALL, "all", false, "Stop ALL Services - including kvcl (virtual-cluster)")

	err = stopCmd.Parse(os.Args[2:])
	if err != nil {
		exitCode = ExitBasicInvocation
		err = fmt.Errorf("error parsing flags: %w", err)
		return
	}

	if !(opts.MCM || opts.MC || opts.CA || opts.ALL) {
		exitCode = ExitBasicInvocation
		stopCmd.PrintDefaults()
		return
	}

	var errs []error
	if opts.MC || opts.ALL {
		err = stopMC(ctx)
		if err != nil {
			exitCode = ExitStopMC
			err = fmt.Errorf("error stopping MC (virtual machine-controller): %w", err)
			errs = append(errs, err)
		}
	}
	if opts.MCM || opts.ALL {
		err = stopMCM(ctx)
		if err != nil {
			exitCode = ExitStopMCM
			err = fmt.Errorf("error stopping MCM (gardener machine-controller-manager): %w", err)
			errs = append(errs, err)
		}
	}
	if opts.CA || opts.ALL {
		err = stopCA(ctx)
		if err != nil {
			exitCode = ExitStopCA
			err = fmt.Errorf("error stopping CA (gardener cluster-autoscaler): %w", err)
			errs = append(errs, err)
		}
	}

	if opts.ALL {
		err = stopKVCL(ctx)
		if err != nil {
			exitCode = ExitStopKVCL
			err = fmt.Errorf("error stopping KVCL (virtual cluster): %w", err)
			errs = append(errs, err)
		}
	}
	err = errors.Join(errs...)
	return
}

func Status(ctx context.Context) (exitCode int, err error) {
	var opts DevOpts
	var errs []error

	statusCmd := flag.NewFlagSet("status", flag.ExitOnError)
	standardUsage := statusCmd.Usage
	statusCmd.Usage = func() {
		w := flag.CommandLine.Output() // may be os.Stderr - but not necessarily
		standardUsage()
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "NOTE: %q with no  option specified checks status of only kvcl (virtual-cluster)", os.Args[1])
	}
	statusCmd.BoolVar(&opts.MCM, "mcm", false, "check status MCM (gardener machine-controller-manager)")
	statusCmd.BoolVar(&opts.MC, "mc", false, "check status MC (virtual machine-controller)")
	statusCmd.BoolVar(&opts.CA, "ca", false, "check status CA (gardener cluster-autoscaler)")
	statusCmd.BoolVar(&opts.ALL, "all", false, "check status of ALL services")

	err = statusCmd.Parse(os.Args[2:])
	if err != nil {
		exitCode = ExitBasicInvocation
		err = fmt.Errorf("error parsing flags: %w", err)
		return
	}

	var pids []int
	if opts.MCM || opts.ALL {
		pids, err = du.FindPidsByName(ctx, MCMName)
		if err != nil {
			errs = append(errs, err)
		}
		if len(pids) > 0 {
			klog.Infof("%s is running with pid(s): %d", MCMName, pids)
		} else {
			klog.Warningf("%s does NOT appear to be running", MCMName)
		}
	}

	if opts.MC || opts.ALL {
		pids, err = du.FindPidsByName(ctx, MCName)
		if err != nil {
			errs = append(errs, err)
		}
		if len(pids) > 0 {
			klog.Infof("%s is running with pid(s): %d", MCName, pids)
		} else {
			klog.Warningf("%s does NOT appear to be running", MCName)
		}
	}

	if opts.CA || opts.ALL {
		pids, err = du.FindPidsByName(ctx, CAName)
		if err != nil {
			errs = append(errs, err)
		}
		if len(pids) > 0 {
			klog.Infof("%s is running with pid(s): %d", CAName, pids)
		} else {
			klog.Warningf("%s does NOT appear to be running", CAName)
		}
	}

	pids, err = du.FindPidsByName(ctx, KVCLName)
	if err != nil {
		errs = append(errs, err)
	}
	if len(pids) > 0 {
		klog.Infof("%s is running with pid(s): %d", KVCLName, pids)
	} else {
		klog.Warningf("%s does NOT appear to be running", KVCLName)
	}

	err = errors.Join(errs...)
	if err != nil {
		exitCode = ExitStatusCheck
	}
	return
}

func DownloadClusterData(ctx context.Context, coord du.ClusterCoordinate) (clusterInfo du.ClusterInfo, err error) {
	clusterInfo, ok := ReadClusterInfo()
	if ok {
		if clusterInfo.ClusterCoordinate != coord {
			klog.Warningf("DownloadClusterData deleting all downloaded files detected change in cluster coordinate from %v->%v",
				clusterInfo.ClusterCoordinate, coord)
			err = os.RemoveAll(Dirs.Gen)
			if err != nil {
				return
			}
			err = os.MkdirAll(Dirs.Gen, 0755)
			if err != nil {
				return
			}
		}
	}
	gctl := du.NewGardenCtl(coord)

	err = du.CreateIfNotExists(Dirs.Spec, 0755)
	if err != nil {
		return
	}
	if !du.Exists(Specs.MachineClasses) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, fmt.Sprintf("kubectl get mcc -oyaml > %s", Specs.MachineClasses))
		if err != nil {
			return
		}
		klog.Infof("Downloaded MachineClasses YAML into %q", Specs.MachineClasses)
	} else {
		klog.Infof("MachineClasses YAML already present at %q", Specs.MachineClasses)
	}

	if !du.Exists(Specs.MachineDeployments) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, fmt.Sprintf("kubectl get mcd -oyaml > %s", Specs.MachineDeployments))
		if err != nil {
			return
		}
		klog.Infof("Downloaded MachineDeployments YAML into %q - skipping download.", Specs.MachineDeployments)
	} else {
		klog.Infof("MachineDeployments YAML already present at %q - skipping download.", Specs.MachineDeployments)
	}

	if !du.Exists(Specs.Worker) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, fmt.Sprintf("kubectl get worker -oyaml > %s", Specs.Worker))
		if err != nil {
			return
		}
		klog.Infof("Downloaded Worker YAML into %q - skipping download.", Specs.Worker)
	} else {
		klog.Infof("Worker YAML already present at %q - skipping download.", Specs.Worker)
	}

	if !du.Exists(Specs.CADeploy) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, fmt.Sprintf("kubectl get deploy cluster-autoscaler -oyaml > %s", Specs.CADeploy))
		if err != nil {
			return
		}
		klog.Infof("Downloaded CA Deploy YAML into %q.", Specs.CADeploy)
	} else {
		klog.Infof("CA Deploy YAML already present at %q - skipping download.", Specs.CADeploy)
	}
	if !du.Exists(Specs.MCMDeploy) {
		_, err = gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, fmt.Sprintf("kubectl get deploy machine-controller-manager -oyaml > %s", Specs.MCMDeploy))
		if err != nil {
			return
		}
		klog.Infof("Downloaded MCM Deploy YAML into %q.", Specs.MCMDeploy)
	} else {
		klog.Infof("MCM Deploy YAML already present at %q - skipping download.", Specs.MCMDeploy)
	}
	if !du.Exists(Specs.CAPriorityExpander) {
		var listCmOut string
		listCmOut, err = gctl.ExecuteCommandOnPlane(ctx, du.DataPlane, "kubectl get cm -n kube-system")
		if err != nil {
			return
		}
		if strings.Contains(listCmOut, "cluster-autoscaler-priority-expander") {
			_, err = gctl.ExecuteCommandOnPlane(ctx, du.DataPlane, fmt.Sprintf("kubectl get cm -n kube-system cluster-autoscaler-priority-expander -oyaml > %s", Specs.CAPriorityExpander))
			if err != nil {
				return
			}
			klog.Infof("Downloaded CA Priority Expander YAML into %q.", Specs.CAPriorityExpander)
		} else {
			klog.Infof("NO CA Priority Expander (%q) configured for %s", "cluster-autoscaler-priority-expander", coord)
		}
	} else {
		klog.Infof("CA Priority Expandder YAML already present at %q - skipping download.", Specs.CAPriorityExpander)
	}

	listSecretsCmd := "kubectl get secrets -o custom-columns=NAME:.metadata.name | grep '^shoot--' | tail +1"
	listSecretsOut, err := gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, listSecretsCmd)
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
		secretSpecPath := path.Join(Dirs.Secret, name+".yaml")
		if du.FileExists(secretSpecPath) {
			klog.Infof("Secret already downloaded at %q - skipping download", secretSpecPath)
			continue
		}
		sb.WriteString("kubectl get secret ")
		sb.WriteString(name)
		sb.WriteString(" -oyaml > ")
		sb.WriteString(secretSpecPath)
		sb.WriteString(" ; ")
	}
	err = os.MkdirAll(Dirs.Secret, 0o755)
	if err != nil {
		return
	}
	if sb.Len() > 0 {
		downloadSecretsCmd := sb.String()
		_, err = gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, downloadSecretsCmd)
		if err != nil {
			return
		}
	}
	shootNamespace, err := gctl.ExecuteCommandOnPlane(ctx, du.ControlPlane, "kubectl config view --minify -o jsonpath='{.contexts[0].context.namespace}'")
	if err != nil {
		return
	}
	klog.Infof("shootNamespace: %q", shootNamespace)
	clusterInfo = du.ClusterInfo{
		ClusterCoordinate: coord,
		ShootNamespace:    shootNamespace,
	}
	err = du.WriteJson(Configs.ClusterInfo, clusterInfo)
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
	sb.WriteString("export KUBECONFIG=")
	sb.WriteString(Configs.LocalKubeConfig)
	sb.WriteString("\n")
	err = os.WriteFile(Configs.EnvScript, []byte(sb.String()), 0o755)
	if err != nil {
		return
	}
	return
}

type StartConfig struct {
	MCMArgs []string
	MCArgs  []string
	CAArgs  []string
}

func GenerateStartConfig() (err error) {
	mcmDeployment, err := du.LoadDeployemntYAML(Specs.MCMDeploy)
	if err != nil {
		return err
	}
	if len(mcmDeployment.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("cannot find mcm container in mcmDeployment %v", mcmDeployment)
	}
	var cfg StartConfig

	mcmContainer := mcmDeployment.Spec.Template.Spec.Containers[0]
	cfg.MCMArgs = mcmContainer.Command[1:]
	replaceKubeConfigOptions(cfg.MCMArgs)
	cfg.MCMArgs = append(cfg.MCMArgs, "--leader-elect=false")

	mcContainer := mcmDeployment.Spec.Template.Spec.Containers[1]
	cfg.MCArgs = mcContainer.Args
	replaceKubeConfigOptions(cfg.MCArgs)
	cfg.MCArgs = append(cfg.MCArgs, "--leader-elect=false")
	err = du.WriteJson(Configs.StartConfig, cfg)
	if err != nil {
		return
	}
	caDeployment, err := du.LoadDeployemntYAML(Specs.CADeploy)
	if err != nil {
		return err
	}
	if len(caDeployment.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("cannot find ca container in caDeployment %v", caDeployment)
	}
	caContainer := caDeployment.Spec.Template.Spec.Containers[0]
	replaceKubeConfigOptions(caContainer.Command)
	cfg.CAArgs = caContainer.Command[1:]
	cfg.CAArgs = append(cfg.CAArgs, "--leader-elect=false")

	err = du.WriteJson(Configs.StartConfig, cfg)
	if err != nil {
		return err
	}
	fmt.Println()
	klog.Infof("NOTE: Generated StartConfig JSON at %q - KINDLY CUSTOMIZE FOR YOUR USE", Configs.StartConfig)
	return
}

func replaceKubeConfigOptions(args []string) {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--kubeconfig=") {
			args[i] = "--kubeconfig=" + Configs.LocalKubeConfig
			continue
		}
		if strings.HasPrefix(arg, "--control-kubeconfig=") {
			args[i] = "--control-kubeconfig=" + Configs.LocalKubeConfig
			continue
		}
		if strings.HasPrefix(arg, "--target-kubeconfig=") {
			args[i] = "--target-kubeconfig=" + Configs.LocalKubeConfig
			continue
		}
		if strings.HasPrefix(arg, "--v=") {
			args[i] = "--v=3"
			continue
		}
	}
	fmt.Printf("args : %v\n", args)
}

func ReadClusterInfo() (clusterInfo du.ClusterInfo, ok bool) {
	ok = false
	p := Configs.ClusterInfo
	if !du.FileExists(p) {
		return
	}
	err := du.ReadJsonInto(p, &clusterInfo)
	if err != nil {
		klog.Errorf("ReadClusterInfo failed to un-marshall %q: %v", p, err)
		return
	}
	ok = true
	return
}

func CopyCRDs(so SetupOpts) (err error) {
	mcmCrdDir := path.Join(so.MCMDir, "kubernetes/crds/")
	if !du.DirExists(mcmCrdDir) {
		err = fmt.Errorf("cannot find MCM CRD's at %q", mcmCrdDir)
		return
	}
	err = os.MkdirAll(Dirs.CRD, 0755)
	if err != nil {
		return err
	}
	err = du.CopyAllFiles(mcmCrdDir, Dirs.CRD)
	if err != nil {
		return err
	}
	return
}

func BuildMCMCABinaries(ctx context.Context, so SetupOpts) (err error) {
	err = du.GoBuild(ctx, so.CADir, "main.go", Bins.CA, so.SkipBuild)
	if err != nil {
		err = fmt.Errorf("error building CA (cluster-autoscaler): %w", err)
		return
	}
	err = du.GoBuild(ctx, so.MCMDir, "cmd/machine-controller-manager/controller_manager.go", Bins.MCM, so.SkipBuild)
	if err != nil {
		err = fmt.Errorf("error building MCM (machine-controller-manager): %w", err)
		return
	}
	err = du.GoBuild(ctx, ".", "cmd/machine-controller/main.go", Bins.MC, so.SkipBuild)
	if err != nil {
		err = fmt.Errorf("error building this project - virtual MC (machine-controller): %w", err)
		return
	}
	return
}

func InstallControlPlane(ctx context.Context) (exitCode int, err error) {
	asBinPath := path.Join(Dirs.Bin, "kube-apiserver")
	if du.FileExists(asBinPath) {
		klog.Infof("InstallControlPlane: %s exists. Assuming control plane binaries are already downloaded by setup-envtest.", asBinPath)
		return
	}
	err = du.GoInstall(ctx, "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest")
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
	err = du.CopyAllFiles(kubeBinAssetsPath, Dirs.Bin)
	if err != nil {
		exitCode = ExitInstallControlPlane
		err = fmt.Errorf("error copying kube binary assets: %w", err)
		return
	}
	return 0, nil
}

func startKVCL(ctx context.Context, cancel context.CancelFunc) (err error) {
	klog.Infof("startKVCL invoked")
	pids, err := du.FindPidsByName(ctx, KVCLName)
	if err != nil {
		return
	}
	if len(pids) > 0 {
		err = fmt.Errorf("%s already started with pids: %v", KVCLName, pids)
		return
	}
	cmd := exec.CommandContext(ctx, Bins.KVCL)
	cmd.Env = append(os.Environ(), "BINARY_ASSETS_DIR="+Dirs.Bin, "KUBECONFIG="+Configs.LocalKubeConfig)
	err = du.LaunchBackgroundCommand(cmd, Logs.KVCL, Pids.KVCL)
	if err != nil {
		return
	}
	kvclWaitSecs := 7
	klog.Infof("Waiting for %d secs after launching KVCL..", kvclWaitSecs)
	<-time.After(time.Second * time.Duration(kvclWaitSecs))
	return
}

func stopKVCL(ctx context.Context) (err error) {
	pids, err := du.FindAndKillProcesses(ctx, KVCLName, "kube-apiserver", "etcd")
	if err != nil {
		return err
	}
	if len(pids) > 0 {
		klog.Infof("stopKVCL killed processes with pids: %v", pids)
	}
	if du.FileExists(Logs.KVCL) {
		err = os.Remove(Logs.KVCL)
	}
	if du.FileExists(Pids.KVCL) {
		err = os.Remove(Pids.KVCL)
	}
	return
}

func initVirtualCluster(ctx context.Context, client *kubernetes.Clientset, shootNamespace string) (err error) {
	var cmd *exec.Cmd
	klog.Infof("initVirtualCluster is applying CRDs from %q", Dirs.CRD)
	cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", Configs.LocalKubeConfig, "apply", "-f", Dirs.CRD)
	out, err := du.InvokeCommand(cmd)
	if err != nil {
		return
	}
	klog.Info(out)
	err = du.CreateNamespace(ctx, client, shootNamespace)
	if err != nil {
		return
	}
	cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", Configs.LocalKubeConfig, "get", "-n", shootNamespace, "mcc")
	out, err = du.InvokeCommand(cmd)
	if strings.TrimSpace(out) == "" {
		cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", Configs.LocalKubeConfig, "apply", "-f", Specs.MachineClasses)
		out, err = du.InvokeCommand(cmd)
		if err != nil {
			return
		}
	}

	cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", Configs.LocalKubeConfig, "get", "-n", shootNamespace, "mcd")
	out, err = du.InvokeCommand(cmd)
	if strings.TrimSpace(out) == "" {
		cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", Configs.LocalKubeConfig, "apply", "-f", Specs.MachineDeployments)
		out, err = du.InvokeCommand(cmd)
		if err != nil {
			return
		}
	}
	return
}

func startMCM(ctx context.Context, client *kubernetes.Clientset, shootNamespace string, cfg StartConfig) (err error) {
	pids, err := du.FindPidsByName(ctx, MCMName)
	if err != nil {
		return
	}
	if len(pids) > 0 {
		klog.Infof("%s already started with pids: %v", MCMName, pids)
		return
	}
	cmd := exec.CommandContext(ctx, Bins.MCM, cfg.MCMArgs...)
	err = du.LaunchBackgroundCommand(cmd, Logs.MCM, Pids.MCM)
	if err != nil {
		return
	}
	return
}

func stopMCM(ctx context.Context) (err error) {
	pids, err := du.FindAndKillProcess(ctx, MCMName)
	if err != nil {
		return err
	}
	if len(pids) > 0 {
		klog.Infof("stopMCM killed MCM process(es) with pid(s): %v", pids)
	}
	if du.FileExists(Pids.MCM) {
		err = os.Remove(Pids.MCM)
	}
	if du.FileExists(Logs.MCM) {
		err = os.Remove(Logs.MCM)
	}
	return
}

func startMC(ctx context.Context, cfg StartConfig) (err error) {
	pids, err := du.FindPidsByName(ctx, MCName)
	if err != nil {
		return
	}
	if len(pids) > 0 {
		klog.Infof("%s already started with pids: %v", MCName, pids)
		return
	}
	cmd := exec.CommandContext(ctx, Bins.MC, cfg.MCArgs...)
	err = du.LaunchBackgroundCommand(cmd, Logs.MC, Pids.MC)
	if err != nil {
		return
	}
	return
}

func stopMC(ctx context.Context) (err error) {
	pids, err := du.FindAndKillProcess(ctx, MCName)
	if err != nil {
		return err
	}
	if len(pids) > 0 {
		klog.Infof("stopMC killed MC process(es) with pid(s): %v", pids)
	}
	if du.FileExists(Pids.MC) {
		err = os.Remove(Pids.MC)
	}
	if du.FileExists(Logs.MC) {
		err = os.Remove(Logs.MC)
	}
	return
}

func startCA(ctx context.Context, client kubernetes.Interface, shootNamespace string, cfg StartConfig) (err error) {
	pids, err := du.FindPidsByName(ctx, CAName)
	if err != nil {
		return
	}
	if len(pids) > 0 {
		//err = fmt.Errorf("%s already started with pids: %v", CAName, pids)
		klog.Infof("%s already started with pids: %v", CAName, pids)
		return
	}
	err = du.CreateUpdateDummyApp(ctx, client, shootNamespace, "machine-controller-manager", 1)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, Bins.CA, cfg.CAArgs...)
	cmd.Env = append(os.Environ(), "TARGET_KUBECONFIG="+Configs.LocalKubeConfig)
	cmd.Env = append(cmd.Env, "CONTROL_KUBECONFIG="+Configs.LocalKubeConfig)
	cmd.Env = append(cmd.Env, "CONTROL_NAMESPACE="+shootNamespace)
	err = du.LaunchBackgroundCommand(cmd, Logs.CA, Pids.CA)
	if err != nil {
		return
	}
	klog.Infof("%s was launched - Kindly check log: %q to see if startup was OK.", CAName, Logs.CA)
	return
}

func stopCA(ctx context.Context) (err error) {
	pids, err := du.FindAndKillProcess(ctx, CAName)
	if err != nil {
		return err
	}
	if len(pids) > 0 {
		klog.Infof("stopCA killed CA process(es) with pids: %v", pids)
	}
	if du.FileExists(Pids.CA) {
		err = os.Remove(Pids.CA)
	}
	if du.FileExists(Logs.CA) {
		err = os.Remove(Logs.CA)
	}
	return
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
		if !du.DirExists(d) {
			err = fmt.Errorf("dir %q for option %q does not exist - kindly check out the project correctly at this path", n, d)
			exitCode = ExitInvalidGoModuleDir
		}
		if !du.FileExists(path.Join(d, "go.mod")) {
			err = fmt.Errorf("%q for option %q is invalid - no go.mod - kindly check out the project correctly at this path", n, d)
			exitCode = ExitInvalidGoModuleDir
			return
		}
	}
	return
}

func InvokeSetupEnvTest(ctx context.Context) (kubeBinAssetsPath string, err error) {
	binPath := path.Join(du.GoBinDir, "setup-envtest")
	cmd := exec.CommandContext(ctx, binPath, "use", "-p", "path")
	kubeBinAssetsPath, err = du.InvokeCommand(cmd)
	if err != nil {
		return
	}
	return
}
