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
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/azuredisk-csi-driver/pkg/apis/azuredisk/v1alpha1"
	diskv1alpha1 "sigs.k8s.io/azuredisk-csi-driver/pkg/apis/azuredisk/v1alpha1"
	azVolumeClientSet "sigs.k8s.io/azuredisk-csi-driver/pkg/apis/client/clientset/versioned"
	diskv1alpha1scheme "sigs.k8s.io/azuredisk-csi-driver/pkg/apis/client/clientset/versioned/scheme"
	"sigs.k8s.io/azuredisk-csi-driver/pkg/azureutils"
	"sigs.k8s.io/azuredisk-csi-driver/pkg/controller/mockclient"
	util "sigs.k8s.io/azuredisk-csi-driver/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	computeDiskURIFormat = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/disks/%s"

	testSubscription  = "12345678-90ab-cedf-1234-567890abcdef"
	testResourceGroup = "test-rg"

	testManagedDiskURI0 = getTestDiskURI(testPersistentVolume0Name)
	testManagedDiskURI1 = getTestDiskURI(testPersistentVolume1Name)

	testNamespace = "test-namespace"

	testNode0Name = "node-0"
	testNode1Name = "node-1"

	testNode1NotFoundError      = k8serrors.NewNotFound(v1.Resource("nodes"), testNode1Name)
	testNode1ServerTimeoutError = k8serrors.NewServerTimeout(v1.Resource("nodes"), testNode1Name, 1)

	testAzDriverNode0 = v1alpha1.AzDriverNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNode0Name,
			Namespace: testNamespace,
		},
	}

	testAzDriverNode1 = v1alpha1.AzDriverNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNode1Name,
			Namespace: testNamespace,
		},
	}

	testNode1Request = createReconcileRequest(testNamespace, testNode1Name)

	testAzDriverNode1NotFoundError = k8serrors.NewNotFound(v1alpha1.Resource("azdrivernodes"), testNode1Name)

	testPersistentVolume0Name = "test-volume-0"
	testPersistentVolume1Name = "test-volume-1"

	testPersistentVolumeClaim0Name = "test-pvc-0"
	testPersistentVolumeClaim1Name = "test-pvc-1"

	testPod0Name = "test-pod-0"
	testPod1Name = "test-pod-1"

	testAzVolume0 = createAzVolume(testPersistentVolume0Name, 1)
	testAzVolume1 = createAzVolume(testPersistentVolume1Name, 1)

	testAzVolume0Request = createReconcileRequest(testNamespace, testPersistentVolume0Name)
	testAzVolume1Request = createReconcileRequest(testNamespace, testPersistentVolume1Name)

	testPrimaryAzVolumeAttachment0Name = azureutils.GetAzVolumeAttachmentName(testPersistentVolume0Name, testNode0Name)
	testPrimaryAzVolumeAttachment1Name = azureutils.GetAzVolumeAttachmentName(testPersistentVolume1Name, testNode0Name)

	testPrimaryAzVolumeAttachment0 = createAzVolumeAttachment(testPersistentVolume0Name, testNode0Name, diskv1alpha1.PrimaryRole)

	testPrimaryAzVolumeAttachment0Request = createReconcileRequest(testNamespace, testPrimaryAzVolumeAttachment0Name)

	testPrimaryAzVolumeAttachment1 = createAzVolumeAttachment(testPersistentVolume1Name, testNode0Name, diskv1alpha1.PrimaryRole)

	testPrimaryAzVolumeAttachment1Request = createReconcileRequest(testNamespace, testPrimaryAzVolumeAttachment1Name)

	testReplicaAzVolumeAttachmentName = azureutils.GetAzVolumeAttachmentName(testPersistentVolume0Name, testNode1Name)

	testReplicaAzVolumeAttachment = createAzVolumeAttachment(testPersistentVolume0Name, testNode1Name, diskv1alpha1.ReplicaRole)

	testReplicaAzVolumeAttachmentRequest = createReconcileRequest(testNamespace, testReplicaAzVolumeAttachmentName)

	testStorageClassName = "test-storage-class"

	testStorageClass = storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: testStorageClassName,
		},
		Provisioner: azureutils.DriverName,
		Parameters: map[string]string{
			azureutils.MaxSharesField:            "2",
			azureutils.MaxMountReplicaCountField: "1",
		},
	}

	testVolumeAttachmentName = "test-attachment"

	testVolumeAttachment = storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name: testVolumeAttachmentName,
		},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: azureutils.DriverName,
			NodeName: testNode0Name,
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &testPersistentVolume0.Name,
			},
		},
	}

	testVolumeAttachmentRequest = reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: testVolumeAttachmentName,
		},
	}

	testPersistentVolume0 = v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPersistentVolume0Name,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       azureutils.DriverName,
					VolumeHandle: fmt.Sprintf(computeDiskURIFormat, testSubscription, testResourceGroup, testPersistentVolume0Name),
				},
			},
			Capacity: v1.ResourceList{
				v1.ResourceStorage: *resource.NewQuantity(util.GiBToBytes(10), resource.DecimalSI),
			},
			ClaimRef: &v1.ObjectReference{
				Namespace: testNamespace,
				Name:      testPersistentVolumeClaim0Name,
			},
			StorageClassName: "",
		},
	}

	testPersistentVolume0Request = reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: testPersistentVolume0Name,
		},
	}

	testPersistentVolume1 = v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: testPersistentVolume1Name,
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       azureutils.DriverName,
					VolumeHandle: fmt.Sprintf(computeDiskURIFormat, testSubscription, testResourceGroup, testPersistentVolume1Name),
				},
			},
			Capacity: v1.ResourceList{
				v1.ResourceStorage: *resource.NewQuantity(util.GiBToBytes(10), resource.DecimalSI),
			},
			ClaimRef: &v1.ObjectReference{
				Namespace: testNamespace,
				Name:      testPersistentVolumeClaim1Name,
			},
			StorageClassName: testStorageClassName,
		},
	}

	testPod0 = createPod(testNamespace, testPod0Name, []string{testPersistentVolumeClaim0Name})

	testPod0Request = createReconcileRequest(testNamespace, testPod0Name)

	testPod1 = createPod(testNamespace, testPod1Name, []string{testPersistentVolumeClaim0Name, testPersistentVolumeClaim1Name})

	testPod1Request = createReconcileRequest(testNamespace, testPod1Name)

	// dead code that could potentially be used in future unit tests
	_ = testAzVolume1Request
	_ = testPrimaryAzVolumeAttachment1Request
)

func getTestDiskURI(pvName string) string {
	return fmt.Sprintf(computeDiskURIFormat, testSubscription, testResourceGroup, pvName)
}

func createReconcileRequest(namespace, name string) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
}

func createAzVolume(pvName string, maxMountReplicaCount int) diskv1alpha1.AzVolume {
	azVolume := diskv1alpha1.AzVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvName,
			Namespace: testNamespace,
		},
		Spec: diskv1alpha1.AzVolumeSpec{
			UnderlyingVolume: pvName,
			CapacityRange: &diskv1alpha1.CapacityRange{
				RequiredBytes: util.GiBToBytes(10),
			},
			MaxMountReplicaCount: maxMountReplicaCount,
		},
	}

	return azVolume
}

func createAzVolumeAttachment(pvName, nodeName string, role diskv1alpha1.Role) diskv1alpha1.AzVolumeAttachment {
	volumeID := getTestDiskURI(pvName)
	azVolumeAttachment := diskv1alpha1.AzVolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      azureutils.GetAzVolumeAttachmentName(pvName, nodeName),
			Namespace: testNamespace,
			Labels: map[string]string{
				azureutils.NodeNameLabel:   nodeName,
				azureutils.VolumeNameLabel: strings.ToLower(pvName),
				azureutils.RoleLabel:       string(role),
			},
		},
		Spec: diskv1alpha1.AzVolumeAttachmentSpec{
			RequestedRole:    role,
			UnderlyingVolume: strings.ToLower(pvName),
			VolumeID:         volumeID,
			NodeName:         nodeName,
		},
	}
	return azVolumeAttachment
}

func createPod(podNamespace, podName string, pvcs []string) corev1.Pod {
	volumes := []v1.Volume{}
	for _, pvc := range pvcs {
		volumes = append(volumes, v1.Volume{
			VolumeSource: v1.VolumeSource{
				CSI: &v1.CSIVolumeSource{
					Driver: azureutils.DriverName,
				},
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc,
				},
			},
		})
	}

	testPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: podNamespace,
			Name:      podName,
		},
		Spec: v1.PodSpec{
			Volumes: volumes,
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
		},
	}

	return testPod
}

func initState(objs ...runtime.Object) (c *SharedState) {
	c = NewSharedState()

	for _, obj := range objs {
		switch target := obj.(type) {
		case *corev1.Pod:
			claims := []string{}
			podKey := getQualifiedName(target.Namespace, target.Name)
			for _, volume := range target.Spec.Volumes {
				if volume.CSI == nil || volume.CSI.Driver != azureutils.DriverName {
					continue
				}
				namespacedClaimName := getQualifiedName(target.Namespace, volume.PersistentVolumeClaim.ClaimName)
				claims = append(claims, namespacedClaimName)
				v, ok := c.claimToPodsMap.Load(namespacedClaimName)
				var pods []string
				if !ok {
					pods = []string{}
				} else {
					pods = v.([]string)
				}
				podExist := false
				for _, pod := range pods {
					if pod == podKey {
						podExist = true
						break
					}
				}
				if !podExist {
					pods = append(pods, podKey)
				}
				c.claimToPodsMap.Store(namespacedClaimName, pods)
			}
			c.podToClaimsMap.Store(podKey, claims)
		case *corev1.PersistentVolume:
			diskName, _ := azureutils.GetDiskNameFromAzureManagedDiskURI(target.Spec.CSI.VolumeHandle)
			azVolumeName := strings.ToLower(diskName)
			claimName := getQualifiedName(target.Spec.ClaimRef.Namespace, target.Spec.ClaimRef.Name)
			c.volumeToClaimMap.Store(azVolumeName, claimName)
			c.claimToVolumeMap.Store(claimName, azVolumeName)
		default:
			continue
		}
	}
	return
}

func splitObjects(objs ...runtime.Object) (diskv1alpha1Objs, kubeObjs []runtime.Object) {
	diskv1alpha1Objs = make([]runtime.Object, 0)
	kubeObjs = make([]runtime.Object, 0)
	for _, obj := range objs {
		if _, _, err := diskv1alpha1scheme.Scheme.ObjectKinds(obj); err == nil {
			diskv1alpha1Objs = append(diskv1alpha1Objs, obj)
		} else {
			kubeObjs = append(kubeObjs, obj)
		}
	}

	return
}

func mockClients(mockClient *mockclient.MockClient, azVolumeClient azVolumeClientSet.Interface, kubeClient kubernetes.Interface) {
	mockClient.EXPECT().
		Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
			switch target := obj.(type) {
			case *diskv1alpha1.AzVolume:
				azVolume, err := azVolumeClient.DiskV1alpha1().AzVolumes(key.Namespace).Get(ctx, key.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				azVolume.DeepCopyInto(target)

			case *diskv1alpha1.AzVolumeAttachment:
				azVolumeAttachment, err := azVolumeClient.DiskV1alpha1().AzVolumeAttachments(key.Namespace).Get(ctx, key.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				azVolumeAttachment.DeepCopyInto(target)

			case *corev1.PersistentVolume:
				pv, err := kubeClient.CoreV1().PersistentVolumes().Get(ctx, key.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				pv.DeepCopyInto(target)

			case *storagev1.VolumeAttachment:
				volumeAttachment, err := kubeClient.StorageV1().VolumeAttachments().Get(ctx, key.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				volumeAttachment.DeepCopyInto(target)

			case *v1.Pod:
				pod, err := kubeClient.CoreV1().Pods(key.Namespace).Get(ctx, key.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				pod.DeepCopyInto(target)

			default:
				gr := schema.GroupResource{
					Group:    target.GetObjectKind().GroupVersionKind().Group,
					Resource: target.GetObjectKind().GroupVersionKind().Kind,
				}
				return k8serrors.NewNotFound(gr, key.Name)
			}

			return nil
		}).
		AnyTimes()
	mockClient.EXPECT().
		Patch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOptions) error {
			data, err := patch.Data(obj)
			if err != nil {
				return err
			}

			switch target := obj.(type) {
			case *diskv1alpha1.AzVolume:
				_, err := azVolumeClient.DiskV1alpha1().AzVolumes(obj.GetNamespace()).Patch(ctx, obj.GetName(), patch.Type(), data, metav1.PatchOptions{})
				if err != nil {
					return err
				}

			case *diskv1alpha1.AzVolumeAttachment:
				_, err := azVolumeClient.DiskV1alpha1().AzVolumeAttachments(obj.GetNamespace()).Patch(ctx, obj.GetName(), patch.Type(), data, metav1.PatchOptions{})
				if err != nil {
					return err
				}

			default:
				gr := schema.GroupResource{
					Group:    target.GetObjectKind().GroupVersionKind().Group,
					Resource: target.GetObjectKind().GroupVersionKind().Kind,
				}
				return k8serrors.NewNotFound(gr, obj.GetName())
			}

			return nil
		}).
		AnyTimes()
	mockClient.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
			switch target := obj.(type) {
			case *diskv1alpha1.AzVolume:
				_, err := azVolumeClient.DiskV1alpha1().AzVolumes(obj.GetNamespace()).Update(ctx, target, metav1.UpdateOptions{})
				if err != nil {
					return err
				}

			case *diskv1alpha1.AzVolumeAttachment:
				_, err := azVolumeClient.DiskV1alpha1().AzVolumeAttachments(obj.GetNamespace()).Update(ctx, target, metav1.UpdateOptions{})
				if err != nil {
					return err
				}

			default:
				gr := schema.GroupResource{
					Group:    target.GetObjectKind().GroupVersionKind().Group,
					Resource: target.GetObjectKind().GroupVersionKind().Kind,
				}
				return k8serrors.NewNotFound(gr, obj.GetName())
			}

			return nil
		}).
		AnyTimes()
	mockClient.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
			options := client.ListOptions{}
			options.ApplyOptions(opts)

			switch target := list.(type) {
			case *diskv1alpha1.AzVolumeAttachmentList:
				azVolumeAttachments, err := azVolumeClient.DiskV1alpha1().AzVolumeAttachments(testNamespace).List(ctx, *options.AsListOptions())
				if err != nil {
					return err
				}

				azVolumeAttachments.DeepCopyInto(target)
			case *diskv1alpha1.AzDriverNodeList:
				azDriverNodes, err := azVolumeClient.DiskV1alpha1().AzDriverNodes(testNamespace).List(ctx, *options.AsListOptions())
				if err != nil {
					return err
				}

				azDriverNodes.DeepCopyInto(target)

			case *corev1.PodList:
				pods, err := kubeClient.CoreV1().Pods("").List(ctx, *options.AsListOptions())
				if err != nil {
					return err
				}

				pods.DeepCopyInto(target)
			}

			return nil
		}).
		AnyTimes()
}