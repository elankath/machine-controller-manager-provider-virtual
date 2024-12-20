package main

import (
	"context"
	"flag"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"os"
)

type Opts struct {
	Kubeconfig     string
	ShootNamespace string
	MCDReplicas    int
}

func main() {
	var o Opts
	flag.StringVar(&o.Kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Kubeconfig file path")
	flag.StringVar(&o.ShootNamespace, "shoot-ns", os.Getenv("SHOOT_NAMESPACE"), "Shoot Namespace")
	flag.IntVar(&o.MCDReplicas, "mcd-replicas", 1, "Patch machine-controller-manager replicas and availableReplicas to given value")
	flag.Parse()
	exitCode, err := execute(context.Background(), o)
	if exitCode == 0 {
		klog.Info("hack completed successfully")
		return
	} else {
		klog.Errorf("hack failed: %v", err)
		os.Exit(exitCode)
	}
}

func execute(ctx context.Context, o Opts) (exitCode int, err error) {
	if o.Kubeconfig == "" {
		return 1, fmt.Errorf("-kubeconfig flag is required")
	}
	if o.ShootNamespace == "" {
		return 1, fmt.Errorf("-shoot-name flag is required")
	}
	// Create a config based on the kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", o.Kubeconfig)
	if err != nil {
		return 2, fmt.Errorf("cannot create rest.Config from kubeconfig %q: %w", o.Kubeconfig, err)
	}
	// Create a Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return 2, fmt.Errorf("cannot create clientset from kubeconfig %q: %w", o.Kubeconfig, err)
	}
	if o.MCDReplicas > 0 {
		err = createUpdateDummyMCD(ctx, clientset, o.ShootNamespace, o.MCDReplicas)
		if err != nil {
			return 3, err
		}
	}
	return 0, nil
}

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
		return fmt.Errorf("hack cannot update replicas to %q for dummy machine-controller-manager deployment: %w", numReplicas, err)
	}
	klog.Infof("hack successfully updated replicas, availableReplicas of dummy machine-controller-manager to %d", numReplicas)
	return nil
}
