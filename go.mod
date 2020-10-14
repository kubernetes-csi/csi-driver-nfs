module github.com/kubernetes-csi/csi-driver-nfs

go 1.13

require (
	github.com/container-storage-interface/spec v1.1.0
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubernetes-csi/csi-lib-utils v0.2.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/spf13/cobra v0.0.3
	golang.org/x/net v0.0.0-20190415100556-4a65cf94b679
	golang.org/x/text v0.3.3 // indirect
	google.golang.org/grpc v1.20.0
	k8s.io/api v0.0.0-20190415132514-c2f1300cac21
	k8s.io/apiextensions-apiserver v0.0.0-20190315093550-53c4693659ed // indirect
	k8s.io/apimachinery v0.0.0-20190415132420-07d458fe0356
	k8s.io/cli-runtime v0.0.0-20190415133733-52015cbe156a // indirect
	k8s.io/cluster-bootstrap v0.0.0-20190415134033-d885a12fbbe4 // indirect
	k8s.io/csi-translation-lib v0.0.0-20190415134207-82f1dfd98d10 // indirect
	k8s.io/kube-aggregator v0.0.0-20190415133304-80ce4e5a0cbc // indirect
	k8s.io/kube-openapi v0.0.0-20190401085232-94e1e7b7574c // indirect
	k8s.io/kubernetes v1.14.1
	k8s.io/utils v0.0.0-20200124190032-861946025e34
)
