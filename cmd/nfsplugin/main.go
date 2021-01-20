/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/kubernetes-csi/csi-driver-nfs/pkg/nfs"

	"k8s.io/klog/v2"
)

var (
	endpoint = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	nodeID   = flag.String("nodeid", "", "node id")
	perm     = flag.String("mount-permissions", "", "mounted folder permissions")
)

func init() {
	_ = flag.Set("logtostderr", "true")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()
	if *nodeID == "" {
		klog.Warning("nodeid is empty")
	}

	handle()
	os.Exit(0)
}

func handle() {
	// Converting string permission representation to *uint32
	var parsedPerm *uint32
	if perm != nil && *perm != "" {
		permu64, err := strconv.ParseUint(*perm, 8, 32)
		if err != nil {
			fmt.Fprintf(os.Stderr, "incorrect mount-permissions value: %q", *perm)
			os.Exit(1)
		}
		permu32 := uint32(permu64)
		parsedPerm = &permu32
	}

	d := nfs.NewNFSdriver(*nodeID, *endpoint, parsedPerm)
	d.Run(false)
}
