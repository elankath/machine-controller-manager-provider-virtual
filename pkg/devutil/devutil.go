package devutil

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
)

var (
	UserHomeDir string
	GoPathDir   string
	GoBinDir    string
)

type GardenerPlane int

const (
	DataPlane    GardenerPlane = 0
	ControlPlane GardenerPlane = 1
)

func init() {
	var err error
	UserHomeDir, err = os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("cannot determine user home directory: %w", err))
	}
	GoPathDir = os.Getenv("GOPATH")
	if strings.TrimSpace(GoPathDir) == "" {
		klog.Infof("GOPATH empty. Defaulting to %s", UserHomeDir)
		GoPathDir = UserHomeDir
	}
	GoBinDir = os.Getenv("GOBIN")
	if strings.TrimSpace(GoBinDir) == "" {
		GoBinDir = filepath.Join(GoPathDir, "bin")
		klog.Infof("GOPATH empty. Defaulting to GOPATH/bin: %s", GoBinDir)
	}
}

func GetGoSourceDir(relProjDir string) string {
	return filepath.Join(GoPathDir, path.Join("src", relProjDir))
}

// GoBuild invokes `go build -o binPath -v -buildvcs=true mainFile` within the given projDir
func GoBuild(ctx context.Context, projDir, mainFile, binPath string, skipBuild bool) error {
	if skipBuild && FileExists(binPath) {
		klog.Infof("GoBuild: Skipping build since %q already exists - kindly delete if you wish to re-build", binPath)
		return nil
	}
	binPathAbs, err := filepath.Abs(binPath)
	if err != nil {
		return fmt.Errorf("cannot get absolute path for path %q: %w", binPath, err)
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-C", projDir, "-o", binPathAbs, "-v", "-buildvcs=true", mainFile)
	_, err = InvokeCommand(cmd)
	if err != nil {
		return err
	}
	if !FileExists(binPath) {
		return fmt.Errorf("did not find binary installed at expected path %q", binPath)
	}
	klog.Infof("Successfully built '%s/%s' and installed binary at %q", projDir, mainFile, binPath)
	return nil
}

// GoInstall invokes `go install toolPathWithVersion`
func GoInstall(ctx context.Context, toolPathWithVersion string) error {
	cmd := exec.CommandContext(ctx, "go", "install", "-v", toolPathWithVersion)
	_, err := InvokeCommand(cmd)
	if err != nil {
		return err
	}
	binPath := path.Join(GoBinDir, "setup-envtest")
	if !FileExists(binPath) {
		err = fmt.Errorf("did not find binary installed at expected path %q", binPath)
		return err
	}
	klog.Infof("Successfully installed binary at %q", binPath)
	return nil
}

func LoadDeployemntYAML(filepath string) (deployment appsv1.Deployment, err error) {
	var data []byte
	data, err = os.ReadFile(filepath)
	if err != nil {
		return
	}
	dcdr := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), len(data))
	err = dcdr.Decode(&deployment)
	return
}

func WriteJson(jpath string, obj any) error {
	data, err := json.MarshalIndent(obj, "", " ")
	if err != nil {
		return err
	}
	err = os.WriteFile(jpath, data, 0755)
	if err != nil {
		return err
	}
	return nil
}

func ReadJsonInto[T any](jpath string, obj *T) (err error) {
	data, err := os.ReadFile(jpath)
	if err != nil {
		klog.Errorf("ReadJson failed to read %q: %v", jpath, err)
		return
	}
	err = json.Unmarshal(data, obj)
	if err != nil {
		klog.Errorf("Readjson failed to un-marshall %q: %v", jpath, err)
		return
	}
	return
}
func ReadJson[T any](jpath string) (obj T, err error) {
	err = ReadJsonInto[T](jpath, &obj)
	return
}

// CopyAllFiles copies all files from srcDir to the destDir
func CopyAllFiles(srcDir, destDir string) error {
	klog.Infof("CopyAllFiles invoked with srcDir: %q, destDir: %q", srcDir, destDir)
	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("source directory error: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", srcDir)
	}

	if err := os.MkdirAll(destDir, 0777); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(srcDir, entry.Name())
		destPath := filepath.Join(destDir, entry.Name())

		if err := CopyFile(sourcePath, destPath); err != nil {
			return err
		}
	}
	return nil
}

func CopyFile(srcPath, dstPath string) error {
	klog.Infof("copying %s to %s", srcPath, dstPath)
	out, err := os.OpenFile(dstPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}

	defer in.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	klog.Infof("copied file %s to %s", srcPath, dstPath)
	return nil
}

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
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

func DirExists(dirpath string) bool {
	fileinfo, err := os.Stat(dirpath)
	if err != nil {
		return false
	}
	if fileinfo.IsDir() {
		return true
	}
	return false
}

func CreateIfNotExists(dir string, perm os.FileMode) error {
	if Exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func CopySymLink(source, dest string) error {
	link, err := os.Readlink(source)
	if err != nil {
		return err
	}
	return os.Symlink(link, dest)
}

func PKill(ctx context.Context, names ...string) error {
	var errs []error
	for _, n := range names {
		cmd := exec.CommandContext(ctx, "pkill", n)
		klog.Infof("Killing process named %q", n)
		outStr, err := InvokeCommand(cmd)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		klog.Info(outStr)
	}
	return errors.Join(errs...)
}

func InvokeCommand(cmd *exec.Cmd) (capturedOutput string, err error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if cmd.Dir != "" {
		klog.Infof("Invoking %q within %q", cmd, cmd.Dir)
	} else {
		klog.Infof("Invoking %q", cmd)
	}
	err = cmd.Run()
	capturedOutput = stdout.String()
	capturedError := stderr.String()
	if err != nil {
		if strings.TrimSpace(capturedError) == "" {
			klog.Warningf("Error running %q: %v", cmd.String(), err)
		} else {
			klog.Warningf("Error running %q: %v", cmd.String(), capturedError)
		}
		err = fmt.Errorf("error invoking %q: %w", cmd, err)
		return
	}
	if strings.TrimSpace(capturedOutput) != "" {
		klog.Infof("Successfully invoked %q, capturedOutput: %s", cmd, capturedOutput)
	} else {
		klog.Infof("Successfully invoked %q", cmd)
	}
	return
}

func FindAndKillProcesses(ctx context.Context, names ...string) (allPids []int, err error) {
	var errs []error
	for _, n := range names {
		var procPids []int
		procPids, err = FindAndKillProcess(ctx, n)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		allPids = append(allPids, procPids...)
	}
	err = errors.Join(errs...)
	return
}

func FindPidsByName(ctx context.Context, name string) (pids []int, err error) {
	cmd := exec.CommandContext(ctx, "ps", "-e", "-o", "pid,comm")
	psOutput, err := cmd.Output()
	if err != nil {
		klog.Errorf("FindProcess could not run ps command: %v", err)
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(psOutput))
	var pid int
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Process the PID and command columns
		pidStr := fields[0]
		commandPath := fields[1]
		commandName := path.Base(commandPath)

		if commandName == name {
			pid, err = strconv.Atoi(pidStr)
			if err != nil {
				err = fmt.Errorf("invalid pid: %s", pidStr)
				return
			}
			pids = append(pids, pid)
		}
	}
	return
}

func FindAndKillProcess(ctx context.Context, name string) (pids []int, err error) {
	pids, err = FindPidsByName(ctx, name)
	if len(pids) == 0 {
		err = fmt.Errorf("failed to find pid(s) for process with name %q", name)
		return
	}
	var proc *os.Process
	for _, pid := range pids {
		proc, err = os.FindProcess(pid)
		if err != nil {
			err = fmt.Errorf("failed to find process with PID %d: %v", pid, err)
			return
		}
		err = proc.Kill()
		if err != nil {
			err = fmt.Errorf("failed to kill process with PID %d: %v", pid, err)
			return
		}
	}
	return
}

func LaunchBackgroundCommand(cmd *exec.Cmd, logPath string, pidPath string) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Start a new process group
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	err = cmd.Start()
	if err != nil {
		return err
	}
	pid := cmd.Process.Pid
	err = os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0666)
	if err != nil {
		return fmt.Errorf("cannot write pid to pidPath %q: %w", pid, err)
	}
	klog.Infof("LaunchCommand: Started %q with pid: %d, logging to: %q", cmd, pid, logPath)
	return nil
}

func LaunchCommand(cmd *exec.Cmd, logPath string, pidPath string, cancel context.CancelFunc) error {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	err = cmd.Start()
	if err != nil {
		return err
	}
	pid := cmd.Process.Pid
	err = os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0666)
	if err != nil {
		return fmt.Errorf("cannot write pid to pidPath %q: %w", pid, err)
	}
	klog.Infof("LaunchCommand: Started %q with pid: %d, logging to: %q", cmd, pid, logPath)
	go func() {
		err = cmd.Wait()
		var exitErr *exec.ExitError
		if err != nil {
			if errors.As(err, &exitErr) {
				klog.Errorf("LaunchCommand: Command %q FAILED with exit code %d", cmd, exitErr.ExitCode())
			} else {
				klog.Warningf("LaunchCommand: Command %q ran into error %s", cmd, err)
			}
			if cancel != nil {
				cancel()
			}
		}
	}()
	return nil
}

func InvokeCommandAndCaptureOutput(cmd *exec.Cmd, outPath string) (err error) {
	capturedOutput, err := InvokeCommand(cmd)
	if err != nil {
		return err
	}
	err = os.WriteFile(outPath, []byte(capturedOutput), 0755)
	klog.Infof("Successfully invoked %q and captured output into outPath: %s", cmd, outPath)
	return
}

type ClusterCoordinate struct {
	Landscape string
	Project   string
	Shoot     string
}

type ClusterInfo struct {
	ClusterCoordinate
	ShootNamespace string
}

type GardenCtl struct {
	Coordinate ClusterCoordinate
}

func NewGardenCtl(coord ClusterCoordinate) *GardenCtl {
	gctl := &GardenCtl{
		Coordinate: coord,
	}
	return gctl
}

var kubeConfigRe = regexp.MustCompile(`export KUBECONFIG='([^']+)'`)

func (g *GardenCtl) GetKubeConfigPath(ctx context.Context, plane GardenerPlane) (kubeConfigPath string, err error) {
	suffix := ""
	if plane == ControlPlane {
		suffix = "--control-plane"
	}
	cmdStr := fmt.Sprintf("eval $(gardenctl kubectl-env zsh) && gardenctl target --garden %s --project %s --shoot %s %s > /dev/null && gardenctl kubectl-env zsh",
		g.Coordinate.Landscape, g.Coordinate.Project, g.Coordinate.Shoot, suffix)
	cmd := exec.CommandContext(ctx, "zsh", "-c", cmdStr)
	cmd.Env = append(os.Environ(), "GCTL_SESSION_ID=dev")
	capturedOut, err := InvokeCommand(cmd)
	if err != nil {
		return
	}
	matches := kubeConfigRe.FindStringSubmatch(capturedOut)
	if len(matches) > 1 {
		kubeConfigPath = matches[1]
		return
	}
	err = fmt.Errorf("cannot get kubeconfig path from output: %s", capturedOut)
	return
}

func (g *GardenCtl) GetShootNamespace(ctx context.Context) (shootNamespace string, err error) {
	return g.ExecuteCommandOnPlane(ctx, ControlPlane, "kubectl config view --minify -o jsonpath='{.contexts[0].context.namespace}'")
}

func (g *GardenCtl) ExecuteCommandOnPlane(ctx context.Context, plane GardenerPlane, kubectlCommand string) (capturedOut string, err error) {
	cmd := exec.CommandContext(ctx, "zsh", "-c", g.genCompositeCommand(plane, kubectlCommand))
	cmd.Env = append(os.Environ(), "GCTL_SESSION_ID=dev")
	capturedOut, err = InvokeCommand(cmd)
	return
}

func (g *GardenCtl) genCompositeCommand(plane GardenerPlane, kubectlCommand string) string {
	suffix := ""
	if plane == ControlPlane {
		suffix = "--control-plane"
	}
	return fmt.Sprintf(
		"eval $(gardenctl kubectl-env zsh)  && gardenctl target --garden %s --project %s --shoot %s %s > /dev/null && %s",
		g.Coordinate.Landscape, g.Coordinate.Project, g.Coordinate.Shoot, suffix, kubectlCommand)
}

func WaitForSignalAndShutdown(ctx context.Context, cancelFunc context.CancelFunc) {
	klog.Info("Waiting until quit signal...")
	quit := make(chan os.Signal, 1)

	/// Use signal.Notify() to listen for incoming SIGINT and SIGTERM signals and relay them to the quit channel.
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	select {
	case s := <-quit:
		klog.Warningf("Received quit signal %q", s.String())
	case <-ctx.Done():
		klog.Warningf("Received cancel %q", ctx.Err())
	}
	cancelFunc()
}

func Pgrep(ctx context.Context, name string) (pids []string) {
	cmd := exec.CommandContext(ctx, "pgrep", name)
	outStr, err := InvokeCommand(cmd)
	if err != nil {
		return
	}
	pids = strings.Split(outStr, "\n")
	return
}

func ReadPidPath(pidPath string) (found bool, pid int, err error) {
	if !FileExists(pidPath) {
		found = false
		return
	}
	var data []byte
	data, err = os.ReadFile(pidPath)
	if err != nil {
		err = fmt.Errorf("cannot read pidPath %q: %w", pidPath, err)
		return
	}
	pid, err = strconv.Atoi(string(data))
	if err != nil {
		err = fmt.Errorf("cannot read %q as pid int pidPath: %w", string(data), err)
		return
	}
	return
}

func CheckProcessRunning(pidPath string) (found bool, pid int, err error) {
	found, pid, err = ReadPidPath(pidPath)
	if err != nil {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		err = fmt.Errorf("err lookup process with pid %d: %w", pid, err)
		return
	}
	err = proc.Signal(syscall.Signal(0))
	if err != nil {
		err = nil
		found = false
	} else {
		found = true
	}
	return
}

func CreateKubeClient(ctx context.Context, kubeConfigPath string) (client *kubernetes.Clientset, err error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		err = fmt.Errorf("cannot create rest.Config from kubeconfig %q: %w", kubeConfigPath, err)
		return
	}
	client, err = kubernetes.NewForConfig(config)
	if err != nil {
		err = fmt.Errorf("cannot create clientset from kubeconfig %q: %w", kubeConfigPath, err)
		return
	}
	return
}

func CreateNamespace(ctx context.Context, client kubernetes.Interface, name string) (err error) {
	nsClient := client.CoreV1().Namespaces()
	nsObj, err := nsClient.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		klog.Infof("Namespace %q already created", name)
		return
	}
	if !apierrors.IsNotFound(err) {
		return
	}
	nsObj = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err = nsClient.Create(ctx, nsObj, metav1.CreateOptions{})
	return
}

func CreateUpdateDummyApp(ctx context.Context, client kubernetes.Interface, shootNamespace, appName string, numReplicas int) error {
	var deployment *appsv1.Deployment
	deploymentClient := client.AppsV1().Deployments(shootNamespace)
	deployment, err := deploymentClient.Get(ctx, appName, metav1.GetOptions{})
	ptrReplicas := ptr.To[int32](int32(numReplicas))
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("Dummy machine-controller manager deployment does not exist, Creating with replicas %d", numReplicas)
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: appName,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptrReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": appName},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": appName},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:    "dummy",
									Image:   "dummy",
									Command: []string{"dummy"},
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
