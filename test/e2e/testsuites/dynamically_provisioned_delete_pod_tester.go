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
	"github.com/kubernetes-csi/csi-driver-nfs/test/e2e/driver"
	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// DynamicallyProvisionedDeletePodTest will provision required StorageClass and Deployment
// Testing if the Pod can write and read to mounted volumes
// Deleting a pod, and again testing if the Pod can write and read to mounted volumes
type DynamicallyProvisionedDeletePodTest struct {
	CSIDriver              driver.DynamicPVTestDriver
	Pod                    PodDetails
	PodCheck               *PodExecCheck
	StorageClassParameters map[string]string
}

type PodExecCheck struct {
	Cmd            []string
	ExpectedString string
}

func (t *DynamicallyProvisionedDeletePodTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	tDeployment, cleanup := t.Pod.SetupDeployment(client, namespace, t.CSIDriver, t.StorageClassParameters)
	// defer must be called here for resources not get removed before using them
	for i := range cleanup {
		defer cleanup[i]()
	}

	ginkgo.By("deploying the deployment")
	tDeployment.Create()

	ginkgo.By("checking that the pod is running")
	tDeployment.WaitForPodReady()

	if t.PodCheck != nil {
		ginkgo.By("checking pod exec")
		tDeployment.Exec(t.PodCheck.Cmd, t.PodCheck.ExpectedString)
	}

	ginkgo.By("deleting the pod for deployment")
	tDeployment.DeletePodAndWait()

	ginkgo.By("checking again that the pod is running")
	tDeployment.WaitForPodReady()

	if t.PodCheck != nil {
		ginkgo.By("checking pod exec")
		// pod will be restarted so expect to see 2 instances of string
		tDeployment.Exec(t.PodCheck.Cmd, t.PodCheck.ExpectedString+t.PodCheck.ExpectedString)
	}
}
