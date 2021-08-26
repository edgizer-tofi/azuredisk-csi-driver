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

package controller

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakev1 "k8s.io/client-go/kubernetes/fake"
	diskv1alpha1 "sigs.k8s.io/azuredisk-csi-driver/pkg/apis/azuredisk/v1alpha1"
	diskfakes "sigs.k8s.io/azuredisk-csi-driver/pkg/apis/client/clientset/versioned/fake"
	"sigs.k8s.io/azuredisk-csi-driver/pkg/azureutils"
	"sigs.k8s.io/azuredisk-csi-driver/pkg/controller/mockclient"
	"sigs.k8s.io/azuredisk-csi-driver/pkg/controller/mockvolumeprovisioner"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewTestAzVolumeController(controller *gomock.Controller, namespace string, objects ...runtime.Object) *ReconcileAzVolume {
	diskv1alpha1Objs, kubeObjs := splitObjects(objects...)

	return &ReconcileAzVolume{
		client:            mockclient.NewMockClient(controller),
		azVolumeClient:    diskfakes.NewSimpleClientset(diskv1alpha1Objs...),
		kubeClient:        fakev1.NewSimpleClientset(kubeObjs...),
		namespace:         namespace,
		volumeProvisioner: mockvolumeprovisioner.NewMockVolumeProvisioner(controller),
	}
}

func mockClientsAndVolumeProvisioner(controller *ReconcileAzVolume) {
	mockClients(controller.client.(*mockclient.MockClient), controller.azVolumeClient, controller.kubeClient)

	controller.volumeProvisioner.(*mockvolumeprovisioner.MockVolumeProvisioner).EXPECT().
		CreateVolume(gomock.Any(), testPersistentVolume0Name, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			volumeName string,
			capacityRange *diskv1alpha1.CapacityRange,
			volumeCapabilities []diskv1alpha1.VolumeCapability,
			parameters map[string]string,
			secrets map[string]string,
			volumeContentSource *diskv1alpha1.ContentVolumeSource,
			accessibilityTopology *diskv1alpha1.TopologyRequirement) (*diskv1alpha1.AzVolumeStatusParams, error) {
			return &diskv1alpha1.AzVolumeStatusParams{
				VolumeID:      testManagedDiskURI0,
				VolumeContext: parameters,
				CapacityBytes: capacityRange.RequiredBytes,
				ContentSource: volumeContentSource,
			}, nil
		}).
		MaxTimes(1)
	controller.volumeProvisioner.(*mockvolumeprovisioner.MockVolumeProvisioner).EXPECT().
		DeleteVolume(gomock.Any(), testManagedDiskURI0, gomock.Any()).
		Return(nil).
		MaxTimes(1)
	controller.volumeProvisioner.(*mockvolumeprovisioner.MockVolumeProvisioner).EXPECT().
		ExpandVolume(gomock.Any(), testManagedDiskURI0, gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			ctx context.Context,
			volumeID string,
			capacityRange *diskv1alpha1.CapacityRange,
			secrets map[string]string) (*diskv1alpha1.AzVolumeStatusParams, error) {
			volumeName, err := azureutils.GetDiskNameFromAzureManagedDiskURI(volumeID)
			if err != nil {
				return nil, err
			}
			azVolume, err := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).Get(context.TODO(), volumeName, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}

			azVolumeStatusParams := azVolume.Status.Detail.ResponseObject.DeepCopy()
			azVolumeStatusParams.CapacityBytes = capacityRange.RequiredBytes

			return azVolumeStatusParams, nil
		}).
		MaxTimes(1)
}

func TestAzVolumeControllerReconcile(t *testing.T) {
	tests := []struct {
		description string
		request     reconcile.Request
		setupFunc   func(*testing.T, *gomock.Controller) *ReconcileAzVolume
		verifyFunc  func(*testing.T, *ReconcileAzVolume, reconcile.Result, error)
	}{
		{
			description: "[Success] Should create a volume when a new AzVolume instance is created.",
			request:     testAzVolume0Request,
			setupFunc: func(t *testing.T, mockCtl *gomock.Controller) *ReconcileAzVolume {
				azVolume := testAzVolume0.DeepCopy()

				azVolume.Status.State = diskv1alpha1.VolumeOperationPending

				controller := NewTestAzVolumeController(
					mockCtl,
					testNamespace,
					azVolume)

				mockClientsAndVolumeProvisioner(controller)

				return controller
			},
			verifyFunc: func(t *testing.T, controller *ReconcileAzVolume, result reconcile.Result, err error) {
				require.NoError(t, err)
				require.False(t, result.Requeue)
				azVolume, err := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).Get(context.TODO(), testPersistentVolume0Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, diskv1alpha1.VolumeCreated, azVolume.Status.State)
			},
		},
		{
			description: "[Success] Should expand a volume when a AzVolume Spec and Status report different sizes.",
			request:     testAzVolume0Request,
			setupFunc: func(t *testing.T, mockCtl *gomock.Controller) *ReconcileAzVolume {
				azVolume := testAzVolume0.DeepCopy()

				azVolume.Status.Detail = &diskv1alpha1.AzVolumeStatusDetail{
					ResponseObject: &diskv1alpha1.AzVolumeStatusParams{
						VolumeID:      testManagedDiskURI0,
						CapacityBytes: azVolume.Spec.CapacityRange.RequiredBytes,
					},
				}
				azVolume.Spec.CapacityRange.RequiredBytes *= 2
				azVolume.Status.State = diskv1alpha1.VolumeCreated

				controller := NewTestAzVolumeController(
					mockCtl,
					testNamespace,
					azVolume)

				mockClientsAndVolumeProvisioner(controller)

				return controller
			},
			verifyFunc: func(t *testing.T, controller *ReconcileAzVolume, result reconcile.Result, err error) {
				require.NoError(t, err)
				require.False(t, result.Requeue)
				azVolume, err := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).Get(context.TODO(), testPersistentVolume0Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, azVolume.Spec.CapacityRange.RequiredBytes, azVolume.Status.Detail.ResponseObject.CapacityBytes)
			},
		},
		{
			description: "[Success] Should delete a volume when a AzVolume is marked for deletion.",
			request:     testAzVolume0Request,
			setupFunc: func(t *testing.T, mockCtl *gomock.Controller) *ReconcileAzVolume {
				azVolume := testAzVolume0.DeepCopy()

				azVolume.Annotations = map[string]string{
					azureutils.VolumeDeleteRequestAnnotation: "cloud-delete-volume",
				}
				azVolume.Finalizers = []string{azureutils.AzVolumeFinalizer}
				azVolume.Status.Detail = &diskv1alpha1.AzVolumeStatusDetail{
					ResponseObject: &diskv1alpha1.AzVolumeStatusParams{
						VolumeID:      testManagedDiskURI0,
						CapacityBytes: azVolume.Spec.CapacityRange.RequiredBytes,
					},
				}
				now := metav1.Time{Time: metav1.Now().Add(-1000)}
				azVolume.ObjectMeta.DeletionTimestamp = &now
				azVolume.Status.State = diskv1alpha1.VolumeCreated

				controller := NewTestAzVolumeController(
					mockCtl,
					testNamespace,
					azVolume)

				mockClientsAndVolumeProvisioner(controller)

				return controller
			},
			verifyFunc: func(t *testing.T, controller *ReconcileAzVolume, result reconcile.Result, err error) {
				require.NoError(t, err)
				require.False(t, result.Requeue)
				azVolume, err := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).Get(context.TODO(), testPersistentVolume0Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, diskv1alpha1.VolumeDeleted, azVolume.Status.State)
			},
		},
		{
			description: "[Success] Should release replica attachments when AzVolume is released.",
			request:     testAzVolume0Request,
			setupFunc: func(t *testing.T, mockCtl *gomock.Controller) *ReconcileAzVolume {
				azVolume := testAzVolume0.DeepCopy()

				azVolume.Status.Detail = &diskv1alpha1.AzVolumeStatusDetail{
					Phase: diskv1alpha1.VolumeReleased,
					ResponseObject: &diskv1alpha1.AzVolumeStatusParams{
						VolumeID:      testManagedDiskURI0,
						CapacityBytes: azVolume.Spec.CapacityRange.RequiredBytes,
					},
				}
				azVolume.Status.State = diskv1alpha1.VolumeCreated

				controller := NewTestAzVolumeController(
					mockCtl,
					testNamespace,
					azVolume,
					&testReplicaAzVolumeAttachment)

				mockClientsAndVolumeProvisioner(controller)

				return controller
			},
			verifyFunc: func(t *testing.T, controller *ReconcileAzVolume, result reconcile.Result, err error) {
				_, localErr := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).Get(context.TODO(), testPersistentVolume0Name, metav1.GetOptions{})
				require.NoError(t, localErr)
				azVolumeAttachments, _ := controller.azVolumeClient.DiskV1alpha1().AzVolumeAttachments(testNamespace).List(context.TODO(), metav1.ListOptions{})
				require.Len(t, azVolumeAttachments.Items, 0)
			},
		},
		{
			description: "[Failure] Should delete volume attachments and requeue when AzVolume is marked for deletion.",
			request:     testAzVolume0Request,
			setupFunc: func(t *testing.T, mockCtl *gomock.Controller) *ReconcileAzVolume {
				azVolume := testAzVolume0.DeepCopy()

				azVolume.Annotations = map[string]string{
					azureutils.VolumeDeleteRequestAnnotation: "cloud-delete-volume",
				}
				azVolume.Finalizers = []string{azureutils.AzVolumeFinalizer}
				azVolume.Status.Detail = &diskv1alpha1.AzVolumeStatusDetail{
					ResponseObject: &diskv1alpha1.AzVolumeStatusParams{
						VolumeID:      testManagedDiskURI0,
						CapacityBytes: azVolume.Spec.CapacityRange.RequiredBytes,
					},
				}
				now := metav1.Time{Time: metav1.Now().Add(-1000)}
				azVolume.ObjectMeta.DeletionTimestamp = &now
				azVolume.Status.State = diskv1alpha1.VolumeCreated

				controller := NewTestAzVolumeController(
					mockCtl,
					testNamespace,
					azVolume,
					&testPrimaryAzVolumeAttachment0,
					&testReplicaAzVolumeAttachment)

				mockClientsAndVolumeProvisioner(controller)

				return controller
			},
			verifyFunc: func(t *testing.T, controller *ReconcileAzVolume, result reconcile.Result, err error) {
				require.Equal(t, status.Errorf(codes.Aborted, "volume deletion requeued until attached azVolumeAttachments are entirely detached..."), err)
				require.True(t, result.Requeue)

				azVolume, err := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).Get(context.TODO(), testPersistentVolume0Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, diskv1alpha1.VolumeCreated, azVolume.Status.State)

				azVolumeAttachments, _ := controller.azVolumeClient.DiskV1alpha1().AzVolumeAttachments(testNamespace).List(context.TODO(), metav1.ListOptions{})
				require.Len(t, azVolumeAttachments.Items, 0)
			},
		},
	}

	for _, test := range tests {
		tt := test
		t.Run(tt.description, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()
			controller := tt.setupFunc(t, mockCtl)
			result, err := controller.Reconcile(context.TODO(), tt.request)
			tt.verifyFunc(t, controller, result, err)
		})
	}
}

func TestAzVolumeControllerRecover(t *testing.T) {
	tests := []struct {
		description string
		setupFunc   func(*testing.T, *gomock.Controller) *ReconcileAzVolume
		verifyFunc  func(*testing.T, *ReconcileAzVolume, error)
	}{
		{
			description: "[Success] Should create AzVolume instances for PersistentVolumes using Azure Disk CSI Driver.",
			setupFunc: func(t *testing.T, mockCtl *gomock.Controller) *ReconcileAzVolume {
				azVolume := testAzVolume0.DeepCopy()

				azVolume.Status.State = diskv1alpha1.VolumeOperationPending

				controller := NewTestAzVolumeController(
					mockCtl,
					testNamespace,
					&testStorageClass,
					&testPersistentVolume0,
					&testPersistentVolume1)

				return controller
			},
			verifyFunc: func(t *testing.T, controller *ReconcileAzVolume, err error) {
				require.NoError(t, err)
				azVolumes, err := controller.azVolumeClient.DiskV1alpha1().AzVolumes(testNamespace).List(context.TODO(), metav1.ListOptions{})
				require.NoError(t, err)
				require.Len(t, azVolumes.Items, 2)
			},
		},
	}

	for _, test := range tests {
		tt := test
		t.Run(tt.description, func(t *testing.T) {
			mockCtl := gomock.NewController(t)
			defer mockCtl.Finish()
			controller := tt.setupFunc(t, mockCtl)
			err := controller.Recover(context.TODO())
			tt.verifyFunc(t, controller, err)
		})
	}
}