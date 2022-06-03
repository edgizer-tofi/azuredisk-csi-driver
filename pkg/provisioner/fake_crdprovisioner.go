/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioner

import (
	"context"

	azdiskv1beta1 "sigs.k8s.io/azuredisk-csi-driver/pkg/apis/azuredisk/v1beta1"
	consts "sigs.k8s.io/azuredisk-csi-driver/pkg/azureconstants"
)

type FakeCrdProvisioner struct {
	CrdProvisioner
	fakeCloudProv *FakeCloudProvisioner
}

func NewFakeCrdProvisioner(cloudProv *FakeCloudProvisioner) (*FakeCrdProvisioner, error) {
	return &FakeCrdProvisioner{
		CrdProvisioner: CrdProvisioner{},
		fakeCloudProv:  cloudProv,
	}, nil
}

func (c *FakeCrdProvisioner) CreateVolume(
	ctx context.Context,
	volumeName string,
	capacityRange *azdiskv1beta1.CapacityRange,
	volumeCapabilities []azdiskv1beta1.VolumeCapability,
	parameters map[string]string,
	secrets map[string]string,
	volumeContentSource *azdiskv1beta1.ContentVolumeSource,
	accessibilityReq *azdiskv1beta1.TopologyRequirement) (*azdiskv1beta1.AzVolumeStatusDetail, error) {
	return c.fakeCloudProv.CreateVolume(ctx, volumeName, capacityRange, volumeCapabilities, parameters, secrets, volumeContentSource, accessibilityReq)
}

func (c *FakeCrdProvisioner) DeleteVolume(ctx context.Context, volumeID string, secrets map[string]string) error {
	return c.fakeCloudProv.DeleteVolume(ctx, volumeID, secrets)
}

func (c *FakeCrdProvisioner) PublishVolume(
	ctx context.Context,
	volumeID string,
	nodeID string,
	volumeCapability *azdiskv1beta1.VolumeCapability,
	readOnly bool,
	secrets map[string]string,
	volumeContext map[string]string) (map[string]string, error) {
	return c.fakeCloudProv.PublishVolume(ctx, volumeID, nodeID, volumeContext)
}

func (c *FakeCrdProvisioner) UnpublishVolume(
	ctx context.Context,
	volumeID string,
	nodeID string,
	secrets map[string]string,
	_ consts.UnpublishMode) error {
	return c.fakeCloudProv.UnpublishVolume(ctx, volumeID, nodeID)
}

func (c *FakeCrdProvisioner) ExpandVolume(
	ctx context.Context,
	volumeID string,
	capacityRange *azdiskv1beta1.CapacityRange,
	secrets map[string]string) (*azdiskv1beta1.AzVolumeStatusDetail, error) {
	return c.fakeCloudProv.ExpandVolume(ctx, volumeID, capacityRange, secrets)
}
