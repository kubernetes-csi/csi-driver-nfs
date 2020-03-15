/*
Copyright 2019 The Kubernetes Authors.
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

package test

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
)

type nfsDriver struct {
	driverInfo testsuites.DriverInfo
	manifests  []string
}

var NFSdriver = InitNFSDriver

type nfsVolume struct {
	serverIP  string
	serverPod *v1.Pod
	f         *framework.Framework
}

// initNFSDriver returns nfsDriver that implements TestDriver interface
func initNFSDriver(name string, manifests ...string) testsuites.TestDriver {
	return &nfsDriver{
		driverInfo: testsuites.DriverInfo{
			Name:        name,
			MaxFileSize: testpatterns.FileSizeLarge,
			SupportedFsType: sets.NewString(
				"", // Default fsType
			),
			Capabilities: map[testsuites.Capability]bool{
				testsuites.CapPersistence: true,
				testsuites.CapExec:        true,
			},
		},
		manifests: manifests,
	}
}

func InitNFSDriver() testsuites.TestDriver {

	return initNFSDriver("nfs.csi.k8s.io",
		"csi-attacher-nfsplugin.yaml",
		"csi-attacher-rbac.yaml",
		"csi-nodeplugin-nfsplugin.yaml",
		"csi-nodeplugin-rbac.yaml")

}

var _ testsuites.TestDriver = &nfsDriver{}
var _ testsuites.PreprovisionedVolumeTestDriver = &nfsDriver{}
var _ testsuites.PreprovisionedPVTestDriver = &nfsDriver{}

func (n *nfsDriver) GetDriverInfo() *testsuites.DriverInfo {
	return &n.driverInfo
}

func (n *nfsDriver) SkipUnsupportedTest(pattern testpatterns.TestPattern) {
	if pattern.VolType == testpatterns.DynamicPV {
		framework.Skipf("NFS Driver does not support dynamic provisioning -- skipping")
	}
}

func (n *nfsDriver) GetPersistentVolumeSource(readOnly bool, fsType string, volume testsuites.TestVolume) (*v1.PersistentVolumeSource, *v1.VolumeNodeAffinity) {
	nv, _ := volume.(*nfsVolume)
	return &v1.PersistentVolumeSource{
		CSI: &v1.CSIPersistentVolumeSource{
			Driver:       n.driverInfo.Name,
			VolumeHandle: "nfs-vol",
			VolumeAttributes: map[string]string{
				"server":   nv.serverIP,
				"share":    "/",
				"readOnly": "true",
			},
		},
	}, nil
}

func (n *nfsDriver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	config := &testsuites.PerTestConfig{
		Driver:    n,
		Prefix:    "nfs",
		Framework: f,
	}

	return config, func() {}
}

func (n *nfsDriver) CreateVolume(config *testsuites.PerTestConfig, volType testpatterns.TestVolType) testsuites.TestVolume {
	f := config.Framework
	cs := f.ClientSet
	ns := f.Namespace

	switch volType {
	case testpatterns.InlineVolume:
		fallthrough
	case testpatterns.PreprovisionedPV:

		//Create nfs server pod
		c := framework.VolumeTestConfig{
			Namespace:          ns.Name,
			Prefix:             "nfs",
			ServerImage:        "gcr.io/kubernetes-e2e-test-images/volume/nfs:1.0",
			ServerPorts:        []int{2049},
			ServerVolumes:      map[string]string{"": "/exports"},
			ServerReadyMessage: "NFS started",
		}
		config.ServerConfig = &c
		serverPod, serverIP := framework.CreateStorageServer(cs, c)

		return &nfsVolume{
			serverIP:  serverIP,
			serverPod: serverPod,
			f:         f,
		}

	case testpatterns.DynamicPV:
		// Do nothing
	default:
		framework.Failf("Unsupported volType:%v is specified", volType)
	}
	return nil
}

func (v *nfsVolume) DeleteVolume() {
	framework.CleanUpVolumeServer(v.f, v.serverPod)
}
