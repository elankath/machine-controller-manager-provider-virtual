/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

This file was copied and modified from the kubernetes/kubernetes project
https://github.com/kubernetes/kubernetes/release-1.8/cmd/kube-controller-manager/controller_manager.go

Modifications Copyright SAP SE or an SAP affiliate company and Gardener contributors
*/

package main

import (
	"context"
	"fmt"
	"github.com/elankath/machine-controller-manager-provider-virtual/pkg/virtual"
	"os"

	//"github.com/gardener/machine-controller-manager-provider-aws/pkg/aws"
	//"github.com/gardener/machine-controller-manager-provider-aws/pkg/spi"
	_ "github.com/gardener/machine-controller-manager/pkg/util/client/metrics/prometheus" // for client metric registration
	"github.com/gardener/machine-controller-manager/pkg/util/provider/app"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/app/options"
	_ "github.com/gardener/machine-controller-manager/pkg/util/reflector/prometheus" // for reflector metric registration
	_ "github.com/gardener/machine-controller-manager/pkg/util/workqueue/prometheus" // for workqueue metric registration
	"github.com/spf13/pflag"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
)

func main() {

	s := options.NewMCServer()
	s.AddFlags(pflag.CommandLine)

	flag.InitFlags()
	logs.InitLogs()
	defer logs.FlushLogs()

	if s.TargetKubeconfig == "" {
		fmt.Errorf("--target-kubeconfig must be provided")
		os.Exit(1)
	}
	if s.Namespace == "" {
		fmt.Errorf("--namespace must be provided")
		os.Exit(2)
	}
	driver, err := virtual.NewDriver(context.Background(), s.TargetKubeconfig, s.Namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}

	if err := app.Run(s, driver); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

}
