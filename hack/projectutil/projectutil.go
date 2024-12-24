package projectutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"k8s.io/klog/v2"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
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

// CopyAllFiles copies all files from srcDir to the destDir
func CopyAllFiles(srcDir, destDir string) error {
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
			klog.Errorf("Error running %q: %v", cmd.String(), err)
		} else {
			klog.Errorf("Error running %q: %v", cmd.String(), capturedError)
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
	//err := gctl.init(ctx)
	//if err != nil {
	//	return nil, err
	//}
	return gctl
}

//func (g *GardenCtl) init(ctx context.Context) (err error) {
//	g.kubeCtlEnvPath, err = gardenCtlEnvZsh(ctx)
//	if err != nil {
//		return
//	}
//	return
//}

func (g *GardenCtl) ExecuteCommandOnPlane(ctx context.Context, plane GardenerPlane, kubectlCommand string) (capturedOut string, err error) {
	cmd := exec.CommandContext(ctx, "zsh", "-c", g.genCompositeCommand(plane, kubectlCommand))
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

//func gardenCtlEnvZsh(ctx context.Context) (envPath string, err error) {
//	var output string
//	cmd := exec.CommandContext(ctx, "gardenctl", "kubectl-env", "zsh")
//	cmd.Env = append(os.Environ(), "GCTL_SESSION_ID=projecutil")
//	output, err = InvokeCommand(cmd)
//	if err != nil {
//		return
//	}
//	var envFile *os.File
//	envFile, err = os.CreateTemp("/tmp", "kubectl-env-*.sh")
//	if err != nil {
//		err = fmt.Errorf("cannot create kubectl-env-X.sh for %q: %w", cmd, err)
//		return
//	}
//	defer envFile.Close()
//	_, err = envFile.WriteString(output)
//	if err != nil {
//		err = fmt.Errorf("error writing to %q: %w", envFile.Name(), err)
//		return
//	}
//	envPath = envFile.Name()
//	klog.Infof("Invoked %q and generated %q", cmd, envPath)
//	return
//}
