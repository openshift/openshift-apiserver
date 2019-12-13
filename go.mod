module github.com/openshift/openshift-apiserver

go 1.13

require (
	github.com/MakeNowJust/heredoc v0.0.0-20170808103936-bb23615498cd
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/certifi/gocertifi v0.0.0-20180905225744-ee1a9a0726d2 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v1.4.2-0.20190327010347-be7ac8be2ae0 // indirect
	github.com/docker/go-units v0.3.3
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/fsouza/go-dockerclient v0.0.0-20171004212419-da3951ba2e9e
	github.com/getsentry/raven-go v0.0.0-20171206001108-32a13797442c // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-openapi/errors v0.19.2
	github.com/go-openapi/spec v0.19.2
	github.com/google/gofuzz v1.0.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/jteeuwen/go-bindata v3.0.7+incompatible
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/openshift/api v3.9.1-0.20191213092759-73a708d0631c+incompatible
	github.com/openshift/apiserver-library-go v0.0.0-20191121105954-72a8df0fc9cc
	github.com/openshift/client-go v0.0.0-20190923180330-3b6373338c9b
	github.com/openshift/library-go v0.0.0-20190924092619-a8c1174d4ee7
	github.com/openshift/source-to-image v1.1.15-0.20190924185145-0b950e4c21af
	github.com/pkg/errors v0.8.1
	github.com/pkg/profile v1.3.0 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	go.uber.org/multierr v1.1.1-0.20180122172545-ddea229ff1df // indirect
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/tools v0.0.0-20191022074931-774d2ec196ee // indirect
	k8s.io/api v0.0.0
	k8s.io/apiextensions-apiserver v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/apiserver v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/code-generator v0.0.0
	k8s.io/component-base v0.0.0
	k8s.io/klog v1.0.0
	k8s.io/kube-aggregator v0.0.0
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	k8s.io/kubectl v0.0.0
	k8s.io/kubernetes v1.16.0
)

replace (
	github.com/docker/distribution => github.com/openshift/docker-distribution v0.0.0-20180925154709-d4c35485a70d
	github.com/docker/docker => github.com/openshift/moby-moby v0.0.0-20190308215630-da810a85109d
	github.com/ghodss/yaml => github.com/ghodss/yaml v0.0.0-20170327235444-0ca9ea5df545
	github.com/golang/glog => github.com/openshift/golang-glog v0.0.0-20190322123450-3c92600d7533
	github.com/moby/buildkit => github.com/dmcgowan/buildkit v0.0.0-20170731200553-da2b9dc7dab9
	github.com/openshift/api => github.com/openshift/api v3.9.1-0.20191213092759-73a708d0631c+incompatible
	github.com/openshift/apiserver-library-go => github.com/openshift/apiserver-library-go v0.0.0-20191121120807-0dbf2b787e04
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20191125132246-f6563a70e19a
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20191209154952-998f403c47c2
	github.com/openshift/source-to-image => github.com/openshift/source-to-image v0.0.0-20191104225151-1877e115164b
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.0.0-20181207105117-505eaef01726
	k8s.io/api => k8s.io/api v0.0.0-20191118180058-457dff596cdb
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20191118182517-73c803b45f89
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191117110801-62c7b2358269
	k8s.io/apiserver => github.com/openshift/kubernetes-apiserver v0.0.0-20191119111452-e7aff81895e5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20191118182912-eec6855a34bc
	k8s.io/client-go => k8s.io/client-go v0.0.0-20191118180547-54e1c278f3e1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20191118184124-5b9e165e7ba4
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20191118183936-9c5eb56f2343
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20191115142644-65da3bb30b8d
	k8s.io/component-base => k8s.io/component-base v0.0.0-20191118180740-52a487cb142d
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20191115184650-140acb75a878
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20191118184306-a36ad8adf00c
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20191118181636-46556a46db92
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20191118183756-3d73fe21d514
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20191118183244-10dda908007b
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20191118183612-02b8e53a15b5
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20191118185145-99276ef8507f
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20191118183426-78a3949c16b0
	k8s.io/kubernetes => k8s.io/kubernetes v0.0.0-20191117110801-fdf93bc4997f
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20191118184543-af3d8bec1770
	k8s.io/metrics => k8s.io/metrics v0.0.0-20191118182722-e73f7a86e353
	k8s.io/node-api => k8s.io/node-api v0.0.0-20191118184730-6a21e3310529
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20191118181922-7fd4aef299c3
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.0.0-20191118183100-3aef6409ab31
	k8s.io/sample-controller => k8s.io/sample-controller v0.0.0-20191118182133-e0ba6ec94108
)
