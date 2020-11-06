package client

import (
	"github.com/kubernetes-csi/csi-proxy/client/apiversion"
)

const (
	// pipePrefix is the prefix for Windows named pipes' names
	pipePrefix = `\\.\\pipe\\`

	// CsiProxyNamedPipePrefix is the prefix for the named pipes the proxy creates.
	// The suffix will be the API group and version,
	// e.g. "\\.\\pipe\\csi-proxy-iscsi-v1", "\\.\\pipe\\csi-proxy-filesystem-v2alpha1", etc.
	csiProxyNamedPipePrefix = "csi-proxy-"
)

func PipePath(apiGroupName string, apiVersion apiversion.Version) string {
	return pipePrefix + csiProxyNamedPipePrefix + apiGroupName + "-" + apiVersion.String()
}
