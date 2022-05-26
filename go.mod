module github.com/openshift/openshift-apiserver

go 1.19

require (
	github.com/MakeNowJust/heredoc v1.0.0
	github.com/containers/image/v5 v5.11.0
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v20.10.7+incompatible
	github.com/docker/go-units v0.4.0
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/go-openapi/errors v0.19.2
	github.com/google/gofuzz v1.1.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/openshift/api v0.0.0-20220218143101-271bd7e1834c
	github.com/openshift/apiserver-library-go v0.0.0-20211209162547-8c11dbc46b6e
	github.com/openshift/build-machinery-go v0.0.0-20211213093930-7e33a7eb4ce3
	github.com/openshift/client-go v0.0.0-20211209144617-7385dd6338e3
	github.com/openshift/library-go v0.0.0-20220210170159-18f172cff934
	github.com/openshift/runtime-utils v0.0.0-20220225175100-8dec0d84fb39
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	go.etcd.io/etcd/client/v3 v3.5.0
	k8s.io/api v0.23.1
	k8s.io/apiextensions-apiserver v0.23.1
	k8s.io/apimachinery v0.23.1
	k8s.io/apiserver v0.23.1
	k8s.io/client-go v0.23.1
	k8s.io/cloud-provider v0.23.1
	k8s.io/code-generator v0.23.1
	k8s.io/component-base v0.23.1
	k8s.io/component-helpers v0.23.1
	k8s.io/klog/v2 v2.30.0
	k8s.io/kube-aggregator v0.23.1
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	k8s.io/kubectl v0.23.1
	k8s.io/kubernetes v1.23.1
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/Microsoft/go-winio v0.4.17 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr v0.0.0-20210826220005-b48c857c3a0e // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/chai2010/gettext-go v0.0.0-20160711120539-c6fed771bfd5 // indirect
	github.com/containers/storage v1.28.1 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.2 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/exponent-io/jsonpath v0.0.0-20151013193312-d6023ce2651d // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-logr/logr v1.2.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/cel-go v0.9.0 // indirect
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.4.1 // indirect
	github.com/moby/term v0.0.0-20210610120745-9d4ed1856297 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/runc v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/profile v1.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.11.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.28.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/russross/blackfriday v1.5.2 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/soheilhy/cmux v0.1.5 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/stretchr/testify v1.7.0 // indirect
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/tmc/grpc-websocket-proxy v0.0.0-20201229170055-e5319fda7802 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	github.com/xlab/treeprint v0.0.0-20181112141820-a009c3971eca // indirect
	go.etcd.io/bbolt v1.3.6 // indirect
	go.etcd.io/etcd/api/v3 v3.5.0 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.0 // indirect
	go.etcd.io/etcd/client/v2 v2.305.0 // indirect
	go.etcd.io/etcd/pkg/v3 v3.5.0 // indirect
	go.etcd.io/etcd/raft/v3 v3.5.0 // indirect
	go.etcd.io/etcd/server/v3 v3.5.0 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0 // indirect
	go.opentelemetry.io/otel v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/trace v0.20.0 // indirect
	go.opentelemetry.io/proto/otlp v0.7.0 // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.19.0 // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/mod v0.4.2 // indirect
	golang.org/x/net v0.0.0-20211209124913-491a49abca63 // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20210831042530-f4d43177bf5e // indirect
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	golang.org/x/tools v0.1.6-0.20210820212750-d4cc65f0b2ff // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210831024726-fe130286e0e2 // indirect
	google.golang.org/grpc v1.40.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/cli-runtime v0.23.1 // indirect
	k8s.io/controller-manager v0.23.1 // indirect
	k8s.io/gengo v0.0.0-20210813121822-485abfe95c7c // indirect
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.25 // indirect
	sigs.k8s.io/json v0.0.0-20211020170558-c049b76a60c6 // indirect
	sigs.k8s.io/kustomize/api v0.10.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.0 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	github.com/docker/distribution => github.com/openshift/docker-distribution v0.0.0-20180925154709-d4c35485a70d
	github.com/docker/docker => github.com/openshift/moby-moby v0.0.0-20190308215630-da810a85109d
	k8s.io/api => k8s.io/api v0.23.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.23.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.23.1
	k8s.io/apiserver => github.com/openshift/kubernetes-apiserver v0.0.0-20211221144435-1fd9911b75fd // openshift-apiserver-4.10-kubernetes-1.23.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.23.1
	k8s.io/client-go => k8s.io/client-go v0.23.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.23.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.23.1
	k8s.io/code-generator => k8s.io/code-generator v0.23.1
	k8s.io/component-base => k8s.io/component-base v0.23.1
	k8s.io/component-helpers => k8s.io/component-helpers v0.23.1
	k8s.io/controller-manager => k8s.io/controller-manager v0.23.1
	k8s.io/cri-api => k8s.io/cri-api v0.23.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.23.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.23.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.23.1
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.23.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.23.1
	k8s.io/kubectl => k8s.io/kubectl v0.23.1
	k8s.io/kubelet => k8s.io/kubelet v0.23.1
	k8s.io/kubernetes => k8s.io/kubernetes v1.23.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.23.1
	k8s.io/metrics => k8s.io/metrics v0.23.1
	k8s.io/mount-utils => k8s.io/mount-utils v0.23.1
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.23.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.23.1
)
