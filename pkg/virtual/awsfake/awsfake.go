package awsfake

import (
	"encoding/json"
	"fmt"
	"github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machinecodes/codes"
	"github.com/gardener/machine-controller-manager/pkg/util/provider/machinecodes/status"
)

// AWSProviderSpec is the spec to be used while parsing the calls.
type AWSProviderSpec struct {
	// APIVersion determines the APIversion for the provider APIs
	APIVersion string `json:"apiVersion,omitempty"`

	// AMI is the disk image version
	AMI string `json:"ami,omitempty"`

	// MachineType contains the EC2 instance type
	MachineType string `json:"machineType,omitempty"`

	// Region contains the AWS region for the machine
	Region string `json:"region,omitempty"`

	// Tags to be specified on the EC2 instances
	Tags map[string]string `json:"tags,omitempty"`
}

// DecodeProviderSpecAndSecret converts request parameters to api.ProviderSpec & api.Secrets
func DecodeProviderSpecAndSecret(machineClass *v1alpha1.MachineClass) (*AWSProviderSpec, error) {
	var (
		providerSpec *AWSProviderSpec
	)

	// Extract providerSpec
	if machineClass == nil {
		return nil, status.Error(codes.InvalidArgument, "MachineClass ProviderSpec is nil")
	}

	err := json.Unmarshal(machineClass.ProviderSpec.Raw, &providerSpec)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return providerSpec, nil
}

// EncodeInstanceID encodes a given instanceID as per it's providerID
func EncodeInstanceID(region, instanceID string) string {
	return fmt.Sprintf("aws:///%s/%s", region, instanceID)
}
