module github.com/duanjiong/test-framework

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/libnetwork v0.8.0-dev.2.0.20190925143933-c8a5fca4a652 // indirect
	github.com/golang/protobuf v1.3.2
	github.com/kubesphere/porter v0.1.2-0.20200601012936-a7170d3b845f
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/osrg/gobgp v0.0.0-20200501041838-df76b278eee6
	github.com/pkg/errors v0.8.1
	github.com/spf13/viper v1.4.0
	github.com/stretchr/testify v1.6.1
	github.com/yunify/qingcloud-cloud-controller-manager v1.4.4
	github.com/yunify/qingcloud-sdk-go v2.0.0-alpha.38+incompatible
	google.golang.org/grpc v1.26.0
	k8s.io/api v1.17.9
	k8s.io/apimachinery v0.18.8
	k8s.io/apiserver v0.18.2
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/component-base v0.18.2
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.18.8
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	sigs.k8s.io/controller-runtime v0.6.0
)

//For develop
replace github.com/kubesphere/porter => ../porter
replace github.com/yunify/qingcloud-cloud-controller-manager => ../qingcloud-cloud-controller-manager

replace (
	k8s.io/api => k8s.io/kubernetes/staging/src/k8s.io/api v0.0.0-20200325144952-9e991415386e
	k8s.io/apiextensions-apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20200325144952-9e991415386e
	k8s.io/apimachinery => k8s.io/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20200325144952-9e991415386e
	k8s.io/apiserver => k8s.io/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20200325144952-9e991415386e
	k8s.io/cli-runtime => k8s.io/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20200325144952-9e991415386e
	k8s.io/client-go => k8s.io/kubernetes/staging/src/k8s.io/client-go v0.0.0-20200325144952-9e991415386e
	k8s.io/cloud-provider => k8s.io/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20200325144952-9e991415386e
	k8s.io/cluster-bootstrap => k8s.io/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20200325144952-9e991415386e
	k8s.io/code-generator => k8s.io/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20200325144952-9e991415386e
	k8s.io/component-base => k8s.io/kubernetes/staging/src/k8s.io/component-base v0.0.0-20200325144952-9e991415386e
	k8s.io/cri-api => k8s.io/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20200325144952-9e991415386e
	k8s.io/csi-translation-lib => k8s.io/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20200325144952-9e991415386e
	k8s.io/kube-aggregator => k8s.io/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20200325144952-9e991415386e
	k8s.io/kube-controller-manager => k8s.io/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20200325144952-9e991415386e
	k8s.io/kube-proxy => k8s.io/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20200325144952-9e991415386e
	k8s.io/kube-scheduler => k8s.io/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20200325144952-9e991415386e
	k8s.io/kubectl => k8s.io/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20200325144952-9e991415386e
	k8s.io/kubelet => k8s.io/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20200325144952-9e991415386e
	k8s.io/legacy-cloud-providers => k8s.io/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20200325144952-9e991415386e
	k8s.io/metrics => k8s.io/kubernetes/staging/src/k8s.io/metrics v0.0.0-20200325144952-9e991415386e
	k8s.io/sample-apiserver => k8s.io/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20200325144952-9e991415386e
	k8s.io/sample-cli-plugin => k8s.io/kubernetes/staging/src/k8s.io/sample-cli-plugin v0.0.0-20200325144952-9e991415386e
	k8s.io/sample-controller => k8s.io/kubernetes/staging/src/k8s.io/sample-controller v0.0.0-20200325144952-9e991415386e
)
