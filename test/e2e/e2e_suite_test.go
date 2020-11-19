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

package e2e

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"time"

	"github.com/kubernetes-csi/csi-driver-nfs/pkg/nfs"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/pborman/uuid"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
)

const (
	kubeconfigEnvVar = "KUBECONFIG"
)

var (
	nodeID                        = os.Getenv("NODE_ID")
	perm                          *uint32
	nfsDriver                     *nfs.Driver
	defaultStorageClassParameters = map[string]string{
		"server": "nfs-server.default.svc.cluster.local",
		"share":  "/",
	}
	controllerServer *nfs.ControllerServer
)

type testCmd struct {
	command  string
	args     []string
	startLog string
	endLog   string
}

var _ = ginkgo.BeforeSuite(func() {
	// k8s.io/kubernetes/test/e2e/framework requires env KUBECONFIG to be set
	// it does not fall back to defaults
	if os.Getenv(kubeconfigEnvVar) == "" {
		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
		os.Setenv(kubeconfigEnvVar, kubeconfig)
	}
	handleFlags()
	framework.AfterReadingAllFlags(&framework.TestContext)

	nfsDriver = nfs.NewNFSdriver(nodeID, fmt.Sprintf("unix:///tmp/csi-%s.sock", uuid.NewUUID().String()), perm)
	controllerServer = nfs.NewControllerServer(nfsDriver)

	// install nfs server
	installNFSServer := testCmd{
		command:  "make",
		args:     []string{"install-nfs-server"},
		startLog: "Installing NFS Server...",
		endLog:   "NFS Server successfully installed",
	}

	e2eBootstrap := testCmd{
		command:  "make",
		args:     []string{"e2e-bootstrap"},
		startLog: "Installing NFS CSI Driver...",
		endLog:   "NFS CSI Driver Installed",
	}
	// todo: Install metrics server once added to this driver

	execTestCmd([]testCmd{installNFSServer, e2eBootstrap})
	go func() {
		nfsDriver.Run()
	}()

})

var _ = ginkgo.AfterSuite(func() {
	createExampleDeployment := testCmd{
		command:  "make",
		args:     []string{"create-example-deployment"},
		startLog: "create example deployments",
		endLog:   "example deployments created",
	}
	execTestCmd([]testCmd{createExampleDeployment})
	// sleep 120s waiting for deployment running complete
	time.Sleep(120 * time.Second)

	nfsLog := testCmd{
		command:  "bash",
		args:     []string{"test/utils/nfs_log.sh"},
		startLog: "===================nfs log===================",
		endLog:   "==================================================",
	}

	e2eTeardown := testCmd{
		command:  "make",
		args:     []string{"e2e-teardown"},
		startLog: "Uninstalling NFS CSI Driver...",
		endLog:   "NFS Driver uninstalled",
	}
	execTestCmd([]testCmd{nfsLog, e2eTeardown})

	// install/uninstall CSI Driver deployment scripts test
	installDriver := testCmd{
		command:  "bash",
		args:     []string{"deploy/install-driver.sh", "master", "local"},
		startLog: "===================install CSI Driver deployment scripts test===================",
		endLog:   "===================================================",
	}
	uninstallDriver := testCmd{
		command:  "bash",
		args:     []string{"deploy/uninstall-driver.sh", "master", "local"},
		startLog: "===================uninstall CSI Driver deployment scripts test===================",
		endLog:   "===================================================",
	}
	execTestCmd([]testCmd{installDriver, uninstallDriver})
})

// handleFlags sets up all flags and parses the command line.
func handleFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
}

func execTestCmd(cmds []testCmd) {
	err := os.Chdir("../..")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer func() {
		err := os.Chdir("test/e2e")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()

	projectRoot, err := os.Getwd()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(strings.HasSuffix(projectRoot, "csi-driver-nfs")).To(gomega.Equal(true))

	for _, cmd := range cmds {
		log.Println(cmd.startLog)
		cmdSh := exec.Command(cmd.command, cmd.args...)
		cmdSh.Dir = projectRoot
		cmdSh.Stdout = os.Stdout
		cmdSh.Stderr = os.Stderr
		err = cmdSh.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		log.Println(cmd.endLog)
	}
}

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}
