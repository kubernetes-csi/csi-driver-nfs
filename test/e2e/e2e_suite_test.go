package e2e

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubernetes-csi/csi-driver-nfs/pkg/nfs"
	testutil "github.com/kubernetes-csi/csi-driver-nfs/test/utils/testutils"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
)

const (
	kubeconfigEnvVar = "KUBECONFIG"
	reportDirEnv     = "ARTIFACTS"
	defaultReportDir = "test/e2e"
)

var (
	nodeID                        = os.Getenv("NODE_ID")
	perm                          *uint32
	nfsDriver                     = nfs.NewNFSdriver(nodeID, "unix:///csi/csi.sock", perm)
	defaultStorageClassParameters = map[string]string{
		"server": "nfs-server.default.svc.cluster.local",
		"share":  "/",
	}
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

	if testutil.IsRunningInProw() {
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
	}
})

var _ = ginkgo.AfterSuite(func() {
	if testutil.IsRunningInProw() {
		e2eTeardown := testCmd{
			command:  "make",
			args:     []string{"e2e-teardown"},
			startLog: "Uninstalling SMB CSI Driver...",
			endLog:   "SMB Driver uninstalled",
		}
		execTestCmd([]testCmd{e2eTeardown})
	}
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
