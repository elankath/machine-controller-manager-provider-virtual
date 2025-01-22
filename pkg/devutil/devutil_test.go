package devutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"os"
	"strings"
	"testing"
)

func TestListScrtControlKubeConfig(t *testing.T) {
	cc, err := getCoordinateFromEnv()
	if err != nil {
		t.Fatal(err)
		return
	}
	ctx := context.Background()
	gctl := NewGardenCtl(cc)
	kubeConfigPath, err := gctl.GetKubeConfigPath(ctx, ControlPlane)
	if err != nil {
		t.Fatal(err)
		return
	}
	t.Logf("kubeConfigPath=%s", kubeConfigPath)
	client, err := CreateKubeClient(ctx, kubeConfigPath)
	if err != nil {
		t.Fatal(err)
		return
	}
	shootNamespace, err := gctl.GetShootNamespace(ctx)
	t.Logf("client=%v, shootNamespace=%s", client, shootNamespace)
	scrtList, err := client.CoreV1().Secrets(shootNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
		return
	}
	scheme := runtime.NewScheme()
	err = corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatal(err)
		return
	}
	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, json.SerializerOptions{Yaml: true, Pretty: true, Strict: true})
	var yamlData bytes.Buffer
	for _, scrt := range scrtList.Items {
		t.Logf("scrt Name=%s", scrt.Name)
		if strings.HasPrefix(scrt.Name, "cloudprovider") {
			scrt.APIVersion = corev1.SchemeGroupVersion.String()
			scrt.Kind = "Secret"
			err = serializer.Encode(&scrt, &yamlData)
			if err != nil {
				err = fmt.Errorf("cannot marshall secret %q due to %w", scrt.Name, err)
				t.Fatal(err)
				return
			}
			t.Logf("yamlData=%s", string(yamlData.Bytes()))
		}

	}
}

func getCoordinateFromEnv() (cc ClusterCoordinate, err error) {
	landscape := os.Getenv("LANDSCAPE")
	if landscape == "" {
		err = errors.New("LANDSCAPE environment variable not set")
		return
	}
	project := os.Getenv("PROJECT")
	if project == "" {
		err = errors.New("PROJECT environment variable not set")
		return
	}
	shoot := os.Getenv("SHOOT")
	if shoot == "" {
		err = errors.New("SHOOT environment variable not set")
		return
	}
	cc.Landscape = landscape
	cc.Project = project
	cc.Shoot = shoot
	return
}
