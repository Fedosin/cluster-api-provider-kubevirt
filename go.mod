module github.com/kubevirt/cluster-api-provider-kubevirt

go 1.13

require (
	github.com/aws/aws-sdk-go v1.15.66
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/logr v0.1.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/mock v1.2.0
	github.com/google/gofuzz v1.1.0
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200417151930-302867dc433b
	github.com/onsi/ginkgo v1.12.0
	github.com/onsi/gomega v1.8.1
	github.com/openshift/machine-api-operator v0.2.1-0.20200402110321-4f3602b96da3

	// kube 1.18
	k8s.io/api v0.18.0
	k8s.io/apimachinery v0.18.0
	k8s.io/client-go v0.18.0
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200327001022-6496210b90e8
	kubevirt.io/client-go v0.20.1
	kubevirt.io/containerized-data-importer v1.8.1-0.20190516083534-83c12eaae2ed
	sigs.k8s.io/controller-runtime v0.5.1-0.20200330174416-a11a908d91e0
	sigs.k8s.io/controller-tools v0.2.9-0.20200331153640-3c5446d407dd
	sigs.k8s.io/yaml v1.2.0
)

replace github.com/openshift/machine-api-operator => github.com/joelspeed/machine-api-operator v0.2.1-0.20200417102748-367ae647375f
