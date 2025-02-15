/*
Copyright 2020 The Kubernetes Authors.

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

package testsuites

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-driver-nfs/pkg/nfs"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/deployment"
	e2ekubectl "k8s.io/kubernetes/test/e2e/framework/kubectl"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2epv "k8s.io/kubernetes/test/e2e/framework/pv"
	imageutils "k8s.io/kubernetes/test/utils/image"
)

const (
	// Some pods can take much longer to get ready due to volume attach/detach latency.
	slowPodStartTimeout = 15 * time.Minute
	// Description that will printed during tests
	failedConditionDescription = "Error status code"
	poll                       = 2 * time.Second
	pollForStringTimeout       = 1 * time.Minute
)

type TestStorageClass struct {
	client       clientset.Interface
	storageClass *storagev1.StorageClass
	namespace    *v1.Namespace
}

// Ideally this would be in "k8s.io/kubernetes/test/e2e/framework"
// Similar to framework.WaitForPodSuccessInNamespaceSlow
var podFailedCondition = func(pod *v1.Pod) (bool, error) {
	switch pod.Status.Phase {
	case v1.PodFailed:
		ginkgo.By("Saw pod failure")
		return true, nil
	case v1.PodSucceeded:
		return true, fmt.Errorf("pod %q succeeded with reason: %q, message: %q", pod.Name, pod.Status.Reason, pod.Status.Message)
	default:
		return false, nil
	}
}

type TestPersistentVolumeClaim struct {
	client                         clientset.Interface
	claimSize                      string
	volumeMode                     v1.PersistentVolumeMode
	storageClass                   *storagev1.StorageClass
	namespace                      *v1.Namespace
	persistentVolume               *v1.PersistentVolume
	persistentVolumeClaim          *v1.PersistentVolumeClaim
	requestedPersistentVolumeClaim *v1.PersistentVolumeClaim
	dataSource                     *v1.TypedLocalObjectReference
}

type TestPod struct {
	client    clientset.Interface
	pod       *v1.Pod
	namespace *v1.Namespace
}

func (t *TestStorageClass) Cleanup(ctx context.Context) {
	framework.Logf("deleting StorageClass %s", t.storageClass.Name)
	err := t.client.StorageV1().StorageClasses().Delete(ctx, t.storageClass.Name, metav1.DeleteOptions{})
	framework.ExpectNoError(err)
}

func (t *TestStorageClass) Create(ctx context.Context) storagev1.StorageClass {
	var err error

	ginkgo.By("creating a StorageClass " + t.storageClass.Name)
	t.storageClass, err = t.client.StorageV1().StorageClasses().Create(ctx, t.storageClass, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	return *t.storageClass
}

func NewTestStorageClass(c clientset.Interface, ns *v1.Namespace, sc *storagev1.StorageClass) *TestStorageClass {
	return &TestStorageClass{
		client:       c,
		storageClass: sc,
		namespace:    ns,
	}
}

func NewTestPersistentVolumeClaimWithDataSource(c clientset.Interface, ns *v1.Namespace, claimSize string, volumeMode VolumeMode, sc *storagev1.StorageClass, dataSource *v1.TypedLocalObjectReference) *TestPersistentVolumeClaim {
	mode := v1.PersistentVolumeFilesystem
	if volumeMode == Block {
		mode = v1.PersistentVolumeBlock
	}
	return &TestPersistentVolumeClaim{
		client:       c,
		claimSize:    claimSize,
		volumeMode:   mode,
		namespace:    ns,
		storageClass: sc,
		dataSource:   dataSource,
	}
}

func NewTestPersistentVolumeClaim(c clientset.Interface, ns *v1.Namespace, claimSize string, volumeMode VolumeMode, sc *storagev1.StorageClass) *TestPersistentVolumeClaim {
	mode := v1.PersistentVolumeFilesystem
	if volumeMode == Block {
		mode = v1.PersistentVolumeBlock
	}
	return &TestPersistentVolumeClaim{
		client:       c,
		claimSize:    claimSize,
		volumeMode:   mode,
		namespace:    ns,
		storageClass: sc,
	}
}

func generatePVC(namespace, storageClassName, claimSize string, volumeMode v1.PersistentVolumeMode, dataSource *v1.TypedLocalObjectReference) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "pvc-",
			Namespace:    namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceName(v1.ResourceStorage): resource.MustParse(claimSize),
				},
			},
			VolumeMode: &volumeMode,
			DataSource: dataSource,
		},
	}
}

func (t *TestPersistentVolumeClaim) Create(ctx context.Context) {
	var err error

	ginkgo.By("creating a PVC")
	storageClassName := ""
	if t.storageClass != nil {
		storageClassName = t.storageClass.Name
	}
	t.requestedPersistentVolumeClaim = generatePVC(t.namespace.Name, storageClassName, t.claimSize, t.volumeMode, t.dataSource)
	t.persistentVolumeClaim, err = t.client.CoreV1().PersistentVolumeClaims(t.namespace.Name).Create(ctx, t.requestedPersistentVolumeClaim, metav1.CreateOptions{})
	framework.ExpectNoError(err)
}

// removeFinalizers is a workaround to solve the problem that PV is stuck at terminating after PVC is deleted.
// Related issue: https://github.com/kubernetes/kubernetes/issues/69697
func (t *TestPersistentVolumeClaim) removeFinalizers(ctx context.Context) {
	pv, err := t.client.CoreV1().PersistentVolumes().Get(ctx, t.persistentVolume.Name, metav1.GetOptions{})
	// Because the pv might be deleted successfully, if so, ignore the error.
	if err != nil && strings.Contains(err.Error(), "not found") {
		return
	}
	framework.ExpectNoError(err)

	pvClone := pv.DeepCopy()

	oldData, err := json.Marshal(pvClone)
	framework.ExpectNoError(err)

	pvClone.ObjectMeta.Finalizers = nil

	newData, err := json.Marshal(pvClone)
	framework.ExpectNoError(err)

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, pvClone)
	framework.ExpectNoError(err)

	_, err = t.client.CoreV1().PersistentVolumes().Patch(ctx, pvClone.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	// Because the pv might be deleted successfully before patched, if so, ignore the error.
	if err != nil && strings.Contains(err.Error(), "not found") {
		return
	}
	framework.ExpectNoError(err)
}

func (t *TestPersistentVolumeClaim) Cleanup(ctx context.Context) {
	framework.Logf("deleting PVC %q/%q", t.namespace.Name, t.persistentVolumeClaim.Name)
	err := e2epv.DeletePersistentVolumeClaim(ctx, t.client, t.persistentVolumeClaim.Name, t.namespace.Name)
	framework.ExpectNoError(err)
	// Wait for the PV to get deleted if reclaim policy is Delete. (If it's
	// Retain, there's no use waiting because the PV won't be auto-deleted and
	// it's expected for the caller to do it.) Technically, the first few delete
	// attempts may fail, as the volume is still attached to a node because
	// kubelet is slowly cleaning up the previous pod, however it should succeed
	// in a couple of minutes.
	if t.persistentVolume.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimDelete {
		if t.persistentVolume.Spec.CSI != nil {
			// only workaround in CSI driver tests
			t.removeFinalizers(ctx)
		}
		ginkgo.By(fmt.Sprintf("waiting for claim's PV %q to be deleted", t.persistentVolume.Name))
		err := e2epv.WaitForPersistentVolumeDeleted(ctx, t.client, t.persistentVolume.Name, 5*time.Second, 10*time.Minute)
		framework.ExpectNoError(err)
	}
	// Wait for the PVC to be deleted
	err = waitForPersistentVolumeClaimDeleted(ctx, t.client, t.persistentVolumeClaim.Name, t.namespace.Name, 5*time.Second, 5*time.Minute)
	framework.ExpectNoError(err)
}

// waitForPersistentVolumeClaimDeleted waits for a PersistentVolumeClaim to be removed from the system until timeout occurs, whichever comes first.
func waitForPersistentVolumeClaimDeleted(ctx context.Context, c clientset.Interface, ns string, pvcName string, Poll, timeout time.Duration) error {
	framework.Logf("Waiting up to %v for PersistentVolumeClaim %s to be removed", timeout, pvcName)
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(Poll) {
		_, err := c.CoreV1().PersistentVolumeClaims(ns).Get(ctx, pvcName, metav1.GetOptions{})
		if err != nil {
			if apierrs.IsNotFound(err) {
				framework.Logf("Claim %q in namespace %q doesn't exist in the system", pvcName, ns)
				return nil
			}
			framework.Logf("Failed to get claim %q in namespace %q, retrying in %v. Error: %v", pvcName, ns, Poll, err)
		}
	}
	return fmt.Errorf("PersistentVolumeClaim %s is not removed from the system within %v", pvcName, timeout)
}

func (t *TestPersistentVolumeClaim) WaitForBound(ctx context.Context) v1.PersistentVolumeClaim {
	var err error

	ginkgo.By(fmt.Sprintf("waiting for PVC to be in phase %q", v1.ClaimBound))
	err = e2epv.WaitForPersistentVolumeClaimPhase(ctx, v1.ClaimBound, t.client, t.namespace.Name, t.persistentVolumeClaim.Name, framework.Poll, framework.ClaimProvisionTimeout)
	framework.ExpectNoError(err)

	ginkgo.By("checking the PVC")
	// Get new copy of the claim
	t.persistentVolumeClaim, err = t.client.CoreV1().PersistentVolumeClaims(t.namespace.Name).Get(ctx, t.persistentVolumeClaim.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)

	return *t.persistentVolumeClaim
}

func (t *TestPersistentVolumeClaim) ValidateProvisionedPersistentVolume(ctx context.Context) {
	var err error

	// Get the bound PersistentVolume
	ginkgo.By("validating provisioned PV")
	t.persistentVolume, err = t.client.CoreV1().PersistentVolumes().Get(ctx, t.persistentVolumeClaim.Spec.VolumeName, metav1.GetOptions{})
	framework.ExpectNoError(err)

	// Check sizes
	expectedCapacity := t.requestedPersistentVolumeClaim.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	claimCapacity := t.persistentVolumeClaim.Spec.Resources.Requests[v1.ResourceName(v1.ResourceStorage)]
	gomega.Expect(claimCapacity.Value()).To(gomega.Equal(expectedCapacity.Value()), "claimCapacity is not equal to requestedCapacity")

	pvCapacity := t.persistentVolume.Spec.Capacity[v1.ResourceName(v1.ResourceStorage)]
	gomega.Expect(pvCapacity.Value()).To(gomega.Equal(expectedCapacity.Value()), "pvCapacity is not equal to requestedCapacity")

	// Check PV properties
	ginkgo.By("checking the PV")
	expectedAccessModes := t.requestedPersistentVolumeClaim.Spec.AccessModes
	gomega.Expect(t.persistentVolume.Spec.AccessModes).To(gomega.Equal(expectedAccessModes))
	gomega.Expect(t.persistentVolume.Spec.ClaimRef.Name).To(gomega.Equal(t.persistentVolumeClaim.ObjectMeta.Name))
	gomega.Expect(t.persistentVolume.Spec.ClaimRef.Namespace).To(gomega.Equal(t.persistentVolumeClaim.ObjectMeta.Namespace))
	// If storageClass is nil, PV was pre-provisioned with these values already set
	if t.storageClass != nil {
		gomega.Expect(t.persistentVolume.Spec.PersistentVolumeReclaimPolicy).To(gomega.Equal(*t.storageClass.ReclaimPolicy))
		gomega.Expect(t.persistentVolume.Spec.MountOptions).To(gomega.Equal(t.storageClass.MountOptions))
		if *t.storageClass.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			gomega.Expect(t.persistentVolume.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values).
				To(gomega.HaveLen(1))
		}
		if len(t.storageClass.AllowedTopologies) > 0 {
			gomega.Expect(t.persistentVolume.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Key).
				To(gomega.Equal(t.storageClass.AllowedTopologies[0].MatchLabelExpressions[0].Key))
			for _, v := range t.persistentVolume.Spec.NodeAffinity.Required.NodeSelectorTerms[0].MatchExpressions[0].Values {
				gomega.Expect(t.storageClass.AllowedTopologies[0].MatchLabelExpressions[0].Values).To(gomega.ContainElement(v))
			}

		}
	}
}

func (t *TestPod) SetNodeSelector(nodeSelector map[string]string) {
	t.pod.Spec.NodeSelector = nodeSelector
}

func (t *TestPod) WaitForFailure(ctx context.Context) {
	err := e2epod.WaitForPodCondition(ctx, t.client, t.namespace.Name, t.pod.Name, failedConditionDescription, slowPodStartTimeout, podFailedCondition)
	framework.ExpectNoError(err)
}

func NewTestPod(c clientset.Interface, ns *v1.Namespace, command string) *TestPod {
	testPod := &TestPod{
		client:    c,
		namespace: ns,
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "nfs-volume-tester-",
			},
			Spec: v1.PodSpec{
				NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
				Containers: []v1.Container{
					{
						Name:         "volume-tester",
						Image:        imageutils.GetE2EImage(imageutils.BusyBox),
						Command:      []string{"/bin/sh"},
						Args:         []string{"-c", command},
						VolumeMounts: make([]v1.VolumeMount, 0),
					},
				},
				RestartPolicy: v1.RestartPolicyNever,
				Volumes:       make([]v1.Volume, 0),
			},
		},
	}

	return testPod
}

func (t *TestPod) SetupRawBlockVolume(pvc *v1.PersistentVolumeClaim, name, devicePath string) {
	volumeDevice := v1.VolumeDevice{
		Name:       name,
		DevicePath: devicePath,
	}
	t.pod.Spec.Containers[0].VolumeDevices = append(t.pod.Spec.Containers[0].VolumeDevices, volumeDevice)

	volume := v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name,
			},
		},
	}
	t.pod.Spec.Volumes = append(t.pod.Spec.Volumes, volume)
}

func (t *TestPod) SetupVolume(pvc *v1.PersistentVolumeClaim, name, mountPath string, readOnly bool) {
	volumeMount := v1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}
	t.pod.Spec.Containers[0].VolumeMounts = append(t.pod.Spec.Containers[0].VolumeMounts, volumeMount)

	volume := v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name,
			},
		},
	}
	t.pod.Spec.Volumes = append(t.pod.Spec.Volumes, volume)
}

func (t *TestPod) Logs(ctx context.Context) ([]byte, error) {
	return podLogs(ctx, t.client, t.pod.Name, t.namespace.Name)
}

func cleanupPodOrFail(ctx context.Context, client clientset.Interface, name, namespace string) {
	framework.Logf("deleting Pod %q/%q", namespace, name)
	body, err := podLogs(ctx, client, name, namespace)
	if err != nil {
		framework.Logf("Error getting logs for pod %s: %v", name, err)
	} else {
		framework.Logf("Pod %s has the following logs: %s", name, body)
	}
	e2epod.DeletePodOrFail(ctx, client, namespace, name)
}

func podLogs(ctx context.Context, client clientset.Interface, name, namespace string) ([]byte, error) {
	return client.CoreV1().Pods(namespace).GetLogs(name, &v1.PodLogOptions{}).Do(ctx).Raw()
}

func (t *TestPod) Create(ctx context.Context) {
	var err error

	t.pod, err = t.client.CoreV1().Pods(t.namespace.Name).Create(ctx, t.pod, metav1.CreateOptions{})
	framework.ExpectNoError(err)
}

func (t *TestPod) WaitForSuccess(ctx context.Context) {
	err := e2epod.WaitForPodSuccessInNamespaceSlow(ctx, t.client, t.pod.Name, t.namespace.Name)
	framework.ExpectNoError(err)
}

func (t *TestPod) WaitForRunning(ctx context.Context) {
	err := e2epod.WaitForPodRunningInNamespace(ctx, t.client, t.pod)
	framework.ExpectNoError(err)
}

func (t *TestPod) Cleanup(ctx context.Context) {
	cleanupPodOrFail(ctx, t.client, t.pod.Name, t.namespace.Name)
}

type TestDeployment struct {
	client     clientset.Interface
	deployment *apps.Deployment
	namespace  *v1.Namespace
	podName    string
}

func NewTestDeployment(c clientset.Interface, ns *v1.Namespace, command string, pvc *v1.PersistentVolumeClaim, volumeName, mountPath string, readOnly bool) *TestDeployment {
	generateName := "nfs-volume-tester-"
	selectorValue := fmt.Sprintf("%s%d", generateName, rand.Int())
	replicas := int32(1)
	testDeployment := &TestDeployment{
		client:    c,
		namespace: ns,
		deployment: &apps.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: generateName,
			},
			Spec: apps.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": selectorValue},
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": selectorValue},
					},
					Spec: v1.PodSpec{
						NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
						Containers: []v1.Container{
							{
								Name:    "volume-tester",
								Image:   imageutils.GetE2EImage(imageutils.BusyBox),
								Command: []string{"/bin/sh"},
								Args:    []string{"-c", command},
								VolumeMounts: []v1.VolumeMount{
									{
										Name:      volumeName,
										MountPath: mountPath,
										ReadOnly:  readOnly,
									},
								},
							},
						},
						RestartPolicy: v1.RestartPolicyAlways,
						Volumes: []v1.Volume{
							{
								Name: volumeName,
								VolumeSource: v1.VolumeSource{
									PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
										ClaimName: pvc.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	return testDeployment
}

func (t *TestDeployment) Create(ctx context.Context) {
	var err error
	t.deployment, err = t.client.AppsV1().Deployments(t.namespace.Name).Create(ctx, t.deployment, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	err = deployment.WaitForDeploymentComplete(t.client, t.deployment)
	framework.ExpectNoError(err)
	pods, err := deployment.GetPodsForDeployment(ctx, t.client, t.deployment)
	framework.ExpectNoError(err)
	// always get first pod as there should only be one
	t.podName = pods.Items[0].Name
}

func (t *TestDeployment) WaitForPodReady(ctx context.Context) {
	pods, err := deployment.GetPodsForDeployment(ctx, t.client, t.deployment)
	framework.ExpectNoError(err)
	// always get first pod as there should only be one
	pod := pods.Items[0]
	t.podName = pod.Name
	err = e2epod.WaitForPodRunningInNamespace(ctx, t.client, &pod)
	framework.ExpectNoError(err)
}

func (t *TestDeployment) PollForStringInPodsExec(command []string, expectedString string) {
	pollForStringInPodsExec(t.namespace.Name, []string{t.podName}, command, expectedString)
}

// Execute the command for all pods in the namespace, looking for expectedString in stdout
func pollForStringInPodsExec(namespace string, pods []string, command []string, expectedString string) {
	ch := make(chan error, len(pods))
	for _, pod := range pods {
		go pollForStringWorker(namespace, pod, command, expectedString, ch)
	}
	errs := make([]error, 0, len(pods))
	for range pods {
		errs = append(errs, <-ch)
	}
	framework.ExpectNoError(utilerrors.NewAggregate(errs), "Failed to find %q in at least one pod's output.", expectedString)
}

func pollForStringWorker(namespace string, pod string, command []string, expectedString string, ch chan<- error) {
	args := append([]string{"exec", pod, "--"}, command...)
	err := wait.PollImmediate(poll, pollForStringTimeout, func() (bool, error) { // nolint
		stdout, err := e2ekubectl.RunKubectl(namespace, args...)
		if err != nil {
			framework.Logf("Error waiting for output %q in pod %q: %v.", expectedString, pod, err)
			return false, nil
		}
		if !strings.Contains(stdout, expectedString) {
			framework.Logf("The stdout did not contain output %q in pod %q, found: %q.", expectedString, pod, stdout)
			return false, nil
		}
		return true, nil
	})
	ch <- err
}

func (t *TestDeployment) DeletePodAndWait(ctx context.Context) {
	framework.Logf("Deleting pod %q in namespace %q", t.podName, t.namespace.Name)
	err := t.client.CoreV1().Pods(t.namespace.Name).Delete(ctx, t.podName, metav1.DeleteOptions{})
	if err != nil {
		if !apierrs.IsNotFound(err) {
			framework.ExpectNoError(fmt.Errorf("pod %q Delete API error: %v", t.podName, err))
		}
		return
	}
	framework.Logf("Waiting for pod %q in namespace %q to be fully deleted", t.podName, t.namespace.Name)
	err = e2epod.WaitForPodNotFoundInNamespace(ctx, t.client, t.podName, t.namespace.Name, e2epod.DefaultPodDeletionTimeout)
	if err != nil {
		framework.ExpectNoError(fmt.Errorf("pod %q error waiting for delete: %w", t.podName, err))
	}
}

func (t *TestDeployment) Cleanup(ctx context.Context) {
	framework.Logf("deleting Deployment %q/%q", t.namespace.Name, t.deployment.Name)
	body, err := t.Logs(ctx)
	if err != nil {
		framework.Logf("Error getting logs for pod %s: %v", t.podName, err)
	} else {
		framework.Logf("Pod %s has the following logs: %s", t.podName, body)
	}
	err = t.client.AppsV1().Deployments(t.namespace.Name).Delete(ctx, t.deployment.Name, metav1.DeleteOptions{})
	framework.ExpectNoError(err)
}

func (t *TestDeployment) Logs(ctx context.Context) ([]byte, error) {
	return podLogs(ctx, t.client, t.podName, t.namespace.Name)
}

func (t *TestPersistentVolumeClaim) ReclaimPolicy() v1.PersistentVolumeReclaimPolicy {
	return t.persistentVolume.Spec.PersistentVolumeReclaimPolicy
}

func (t *TestPersistentVolumeClaim) WaitForPersistentVolumePhase(ctx context.Context, phase v1.PersistentVolumePhase) {
	err := e2epv.WaitForPersistentVolumePhase(ctx, phase, t.client, t.persistentVolume.Name, 5*time.Second, 10*time.Minute)
	framework.ExpectNoError(err)
}

func (t *TestPersistentVolumeClaim) DeleteBoundPersistentVolume(ctx context.Context) {
	ginkgo.By(fmt.Sprintf("deleting PV %q", t.persistentVolume.Name))
	err := e2epv.DeletePersistentVolume(ctx, t.client, t.persistentVolume.Name)
	framework.ExpectNoError(err)
	ginkgo.By(fmt.Sprintf("waiting for claim's PV %q to be deleted", t.persistentVolume.Name))
	err = e2epv.WaitForPersistentVolumeDeleted(ctx, t.client, t.persistentVolume.Name, 5*time.Second, 10*time.Minute)
	framework.ExpectNoError(err)
}

func (t *TestPersistentVolumeClaim) DeleteBackingVolume(cs *nfs.ControllerServer) {
	volumeID := t.persistentVolume.Spec.CSI.VolumeHandle
	ginkgo.By(fmt.Sprintf("deleting nfs volume %q", volumeID))
	req := &csi.DeleteVolumeRequest{
		VolumeId: volumeID,
	}
	_, err := cs.DeleteVolume(context.Background(), req)
	if err != nil {
		ginkgo.Fail(fmt.Sprintf("could not delete volume %q: %v", volumeID, err))
	}
}

func (t *TestPod) SetupVolumeMountWithSubpath(pvc *v1.PersistentVolumeClaim, name, mountPath string, subpath string, readOnly bool) {
	volumeMount := v1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		SubPath:   subpath,
		ReadOnly:  readOnly,
	}

	t.pod.Spec.Containers[0].VolumeMounts = append(t.pod.Spec.Containers[0].VolumeMounts, volumeMount)

	volume := v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.Name,
			},
		},
	}

	t.pod.Spec.Volumes = append(t.pod.Spec.Volumes, volume)
}

func (t *TestPod) SetupCSIInlineVolume(name, mountPath, server, share, mountOptions string, readOnly bool) {
	volumeMount := v1.VolumeMount{
		Name:      name,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}
	t.pod.Spec.Containers[0].VolumeMounts = append(t.pod.Spec.Containers[0].VolumeMounts, volumeMount)

	volume := v1.Volume{
		Name: name,
		VolumeSource: v1.VolumeSource{
			CSI: &v1.CSIVolumeSource{
				Driver: nfs.DefaultDriverName,
				VolumeAttributes: map[string]string{
					"server":       server,
					"share":        share,
					"mountOptions": mountOptions,
				},
				ReadOnly: &readOnly,
			},
		},
	}
	t.pod.Spec.Volumes = append(t.pod.Spec.Volumes, volume)
}
