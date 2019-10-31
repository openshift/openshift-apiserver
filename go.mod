module github.com/openshift/openshift-apiserver

go 1.13

require (
	github.com/MakeNowJust/heredoc v0.0.0-20170808103936-bb23615498cd
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/certifi/gocertifi v0.0.0-20180905225744-ee1a9a0726d2 // indirect
	github.com/containerd/continuity v0.0.0-20190827140505-75bee3e2ccb6 // indirect
	github.com/coreos/etcd v3.3.15+incompatible
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v1.4.2-0.20190327010347-be7ac8be2ae0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.3.3
	github.com/docker/libnetwork v0.8.0-dev.2.0.20190731215715-7f13a5c99f4b // indirect
	github.com/docker/libtrust v0.0.0-20150526203908-9cbd2a1374f4 // indirect
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/fsouza/go-dockerclient v0.0.0-20171004212419-da3951ba2e9e
	github.com/getsentry/raven-go v0.0.0-20171206001108-32a13797442c // indirect
	github.com/ghodss/yaml v0.0.0-20180820084758-c7ce16629ff4
	github.com/go-openapi/errors v0.19.2
	github.com/go-openapi/spec v0.19.2
	github.com/google/btree v1.0.0 // indirect
	github.com/google/gofuzz v1.0.0
	github.com/grpc-ecosystem/go-grpc-middleware v1.0.1-0.20190222133341-cfaf5686ec79 // indirect
	github.com/hashicorp/golang-lru v0.5.1
	github.com/jteeuwen/go-bindata v3.0.7+incompatible
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1
	github.com/opencontainers/runc v1.0.0-rc4.0.20170825135527-4d6e6720a7c8 // indirect
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/apiserver-library-go v0.0.0-20190924100232-623481f36143
	github.com/openshift/client-go v0.0.0-20190923180330-3b6373338c9b
	github.com/openshift/library-go v0.0.0-20190924092619-a8c1174d4ee7
	github.com/openshift/source-to-image v1.1.15-0.20190924185145-0b950e4c21af
	github.com/pkg/profile v1.3.0 // indirect
	github.com/prometheus/client_golang v1.1.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.3
	go.uber.org/atomic v1.3.3-0.20181018215023-8dc6146f7569 // indirect
	go.uber.org/multierr v1.1.1-0.20180122172545-ddea229ff1df // indirect
	go.uber.org/zap v1.9.2-0.20180814183419-67bc79d13d15 // indirect
	golang.org/x/net v0.0.0-20190812203447-cdfb69ac37fc
	k8s.io/api v0.0.0
	k8s.io/apiextensions-apiserver v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/apiserver v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/code-generator v0.0.0
	k8s.io/component-base v0.0.0
	k8s.io/klog v0.4.0
	k8s.io/kube-aggregator v0.0.0
	k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
	k8s.io/kubectl v0.0.0
	k8s.io/kubernetes v1.16.0
)

replace (
	github.com/docker/distribution => github.com/openshift/docker-distribution v0.0.0-20180925154709-d4c35485a70d
	github.com/golang/glog => github.com/openshift/golang-glog v0.0.0-20190322123450-3c92600d7533
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191015104839-b9365caf92e3
	github.com/openshift/apiserver-library-go => github.com/openshift/apiserver-library-go v0.0.0-20190924100232-623481f36143
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20191001081553-3b0e988f8cb0
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20191014125628-e7604f697814
	github.com/openshift/source-to-image => github.com/openshift/source-to-image v0.0.0-20190924185145-0b950e4c21af
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2
	k8s.io/api => k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190918161926-8f644eb6e783
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/apiserver => github.com/openshift/kubernetes-apiserver v0.0.0-20191031131608-0caf1f34c3d9
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190918162238-f783a3654da8
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190918163234-a9c1f33e9fb9
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20190918163108-da9fdfce26bb
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190912054826-cd179ad6a269
	k8s.io/component-base => k8s.io/component-base v0.0.0-20190918160511-547f6c5d7090
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190828162817-608eb1dad4ac
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20190918163402-db86a8c7bb21
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190918161219-8c8f079fddc3
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20190918162944-7a93a0ddadd8
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20190918162534-de037b596c1e
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20190918162820-3b5c1246eb18
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20190918164019-21692a0861df
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20190918162654-250a1838aa2c
	k8s.io/kubernetes => github.com/openshift/kubernetes v1.16.0-beta.0.0.20190926205813-ab72ed558cb1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20190918163543-cfa506e53441
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190918162108-227c654b2546
	k8s.io/node-api => k8s.io/node-api v0.0.0-20190918163711-2299658ad911
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20190918161442-d4c9c65c82af
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.0.0-20190918162410-e45c26d066f2
	k8s.io/sample-controller => k8s.io/sample-controller v0.0.0-20190918161628-92eb3cb7496c
)

replace k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190816220812-743ec37842bf
