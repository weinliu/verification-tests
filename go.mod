module github.com/openshift/openshift-tests-private

go 1.18

require (
	cloud.google.com/go/logging v1.5.0
	cloud.google.com/go/storage v1.22.1
	github.com/Azure/azure-sdk-for-go v55.0.0+incompatible
	github.com/Azure/azure-storage-blob-go v0.15.0
	github.com/Azure/go-autorest/autorest v0.11.27
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.11
	github.com/RangelReale/osincli v0.0.0-20160924135400-fababb0555f2
	github.com/aws/aws-sdk-go v1.43.10
	github.com/aws/aws-sdk-go-v2 v1.13.0
	github.com/aws/aws-sdk-go-v2/config v1.11.1
	github.com/aws/aws-sdk-go-v2/credentials v1.6.5
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.11.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.24.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/docker v20.10.17+incompatible
	github.com/fsouza/go-dockerclient v0.0.0-20171004212419-da3951ba2e9e
	github.com/ghodss/yaml v1.0.0
	github.com/go-git/go-git/v5 v5.4.2
	github.com/google/go-github v17.0.0+incompatible
	github.com/gophercloud/gophercloud v0.1.0
	github.com/mattn/go-sqlite3 v1.14.6
	github.com/onsi/ginkgo/v2 v2.1.5-0.20220909190140-b488ab12695a
	github.com/onsi/gomega v1.20.1
	github.com/openshift/api v0.0.0-20220831183848-09c070622e2c
	github.com/openshift/client-go v0.0.0-20220831193253-4950ae70c8ea
	github.com/openshift/library-go v0.0.0-20220831200629-330d9097d0f0
	github.com/pborman/uuid v1.2.0
	github.com/prometheus/client_golang v1.12.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.32.1
	github.com/rs/zerolog v1.27.0
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.11.0
	github.com/tidwall/pretty v1.2.0
	github.com/tidwall/sjson v1.2.3
	github.com/vmware/govmomi v0.27.4
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e
	golang.org/x/oauth2 v0.0.0-20220608161450-d0670ef3b1eb
	google.golang.org/api v0.84.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.25.0
	k8s.io/apimachinery v0.25.0
	k8s.io/apiserver v0.25.0
	k8s.io/cli-runtime v0.25.0
	k8s.io/client-go v0.25.0
	k8s.io/component-base v0.25.0
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.70.1
	k8s.io/kubectl v0.25.0
	k8s.io/kubernetes v1.25.0
	k8s.io/legacy-cloud-providers v0.25.0
	k8s.io/utils v0.0.0-20220728103510-ee6ede2d64ed
	sigs.k8s.io/yaml v1.2.0
)

require (
	cloud.google.com/go v0.102.1 // indirect
	cloud.google.com/go/compute v1.7.0 // indirect
	cloud.google.com/go/iam v0.3.0 // indirect
	github.com/Azure/azure-pipeline-go v0.2.3 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.20 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.5 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/mocks v0.4.2 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.1.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/GoogleCloudPlatform/k8s-cloud-provider v1.18.1-0.20220218231025-f11817397a1b // indirect
	github.com/JeffAshton/win_pdh v0.0.0-20161109143554-76bb4ee9f0ab // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.4.17 // indirect
	github.com/Microsoft/hcsshim v0.8.22 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20210428141323-04723f9f07d7 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/armon/circbuf v0.0.0-20150827004946-bbbad097214e // indirect
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.8.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.11.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.7.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.12.0 // indirect
	github.com/aws/smithy-go v1.10.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/checkpoint-restore/go-criu/v5 v5.3.0 // indirect
	github.com/cilium/ebpf v0.7.0 // indirect
	github.com/container-storage-interface/spec v1.6.0 // indirect
	github.com/containerd/cgroups v1.0.1 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/containerd v1.4.9 // indirect
	github.com/containerd/ttrpc v1.0.2 // indirect
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.3-0.20220203105225-a9a7ef127534 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/docker/distribution v2.8.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/docker/libnetwork v0.0.0-20190731215715-7f13a5c99f4b // indirect
	github.com/emicklei/go-restful/v3 v3.8.0 // indirect
	github.com/emirpasic/gods v1.12.0 // indirect
	github.com/euank/go-kmsg-parser v2.0.0+incompatible // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-errors/errors v1.0.1 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/go-ozzo/ozzo-validation v3.5.0+incompatible // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.2.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/cadvisor v0.45.0 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.2.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.0.0-20220520183353-fd19c99a87aa // indirect
	github.com/googleapis/gax-go/v2 v2.4.0 // indirect
	github.com/googleapis/go-type-adapters v1.0.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/heketi/heketi v10.3.0+incompatible // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/karrick/godirwalk v1.16.1 // indirect
	github.com/kevinburke/ssh_config v0.0.0-20201106050909-4977a11b4351 // indirect
	github.com/libopenstorage/openstorage v1.0.0 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lithammer/dedent v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-ieproxy v0.0.1 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mindprince/gonvml v0.0.0-20190828220739-9ebdce4bb989 // indirect
	github.com/mistifyio/go-zfs v2.1.2-0.20190413222219-f784269be439+incompatible // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.4.1 // indirect
	github.com/moby/ipvs v1.0.1 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/moby/sys/mount v0.3.3 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170603005431-491d3605edfb // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mrunalp/fileutils v0.5.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runc v1.1.3 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/opencontainers/selinux v1.10.0 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/profile v1.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rubiojr/go-vhd v0.0.0-20200706105327-02e210299021 // indirect
	github.com/russross/blackfriday v1.5.2 // indirect
	github.com/seccomp/libseccomp-golang v0.9.2-0.20220502022130-f33da4d89646 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/vishvananda/netlink v1.1.0 // indirect
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae // indirect
	github.com/xanzy/ssh-agent v0.3.0 // indirect
	github.com/xlab/treeprint v1.1.0 // indirect
	go.etcd.io/etcd/api/v3 v3.5.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.5.4 // indirect
	go.etcd.io/etcd/client/v3 v3.5.4 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/github.com/emicklei/go-restful/otelrestful v0.20.0 // indirect
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
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4 // indirect
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8 // indirect
	golang.org/x/xerrors v0.0.0-20220609144429-65e65417b02f // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220617124728-180714bec0ad // indirect
	google.golang.org/grpc v1.47.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/gcfg.v1 v1.2.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gotest.tools v2.2.0+incompatible // indirect
	k8s.io/apiextensions-apiserver v0.25.0 // indirect
	k8s.io/cloud-provider v0.0.0 // indirect
	k8s.io/component-helpers v0.25.0 // indirect
	k8s.io/cri-api v0.0.0 // indirect
	k8s.io/csi-translation-lib v0.0.0 // indirect
	k8s.io/kube-openapi v0.0.0-20220803162953-67bda5d908f1 // indirect
	k8s.io/kube-proxy v0.0.0 // indirect
	k8s.io/kube-scheduler v0.0.0 // indirect
	k8s.io/kubelet v0.0.0 // indirect
	k8s.io/mount-utils v0.0.0 // indirect
	k8s.io/pod-security-admission v0.25.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.32 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/kube-storage-version-migrator v0.0.4 // indirect
	sigs.k8s.io/kustomize/api v0.12.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.13.9 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	bitbucket.org/ww/goautoneg => github.com/munnerz/goautoneg v0.0.0-20120707110453-a547fc61f48d
	github.com/google/cadvisor => github.com/google/cadvisor v0.45.0
	github.com/jteeuwen/go-bindata => github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
	github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.0.0-20221018202518-d6fe52fe5a42
	k8s.io/api => github.com/openshift/kubernetes/staging/src/k8s.io/api v0.0.0-20221020223141-4bd0702b5791
	k8s.io/apiextensions-apiserver => github.com/openshift/kubernetes/staging/src/k8s.io/apiextensions-apiserver v0.0.0-20221020223141-4bd0702b5791
	k8s.io/apimachinery => github.com/openshift/kubernetes/staging/src/k8s.io/apimachinery v0.0.0-20221020223141-4bd0702b5791
	k8s.io/apiserver => github.com/openshift/kubernetes/staging/src/k8s.io/apiserver v0.0.0-20221020223141-4bd0702b5791
	k8s.io/cli-runtime => github.com/openshift/kubernetes/staging/src/k8s.io/cli-runtime v0.0.0-20221020223141-4bd0702b5791
	k8s.io/client-go => github.com/openshift/kubernetes/staging/src/k8s.io/client-go v0.0.0-20221020223141-4bd0702b5791
	k8s.io/cloud-provider => github.com/openshift/kubernetes/staging/src/k8s.io/cloud-provider v0.0.0-20221020223141-4bd0702b5791
	k8s.io/cluster-bootstrap => github.com/openshift/kubernetes/staging/src/k8s.io/cluster-bootstrap v0.0.0-20221020223141-4bd0702b5791
	k8s.io/code-generator => github.com/openshift/kubernetes/staging/src/k8s.io/code-generator v0.0.0-20221020223141-4bd0702b5791
	k8s.io/component-base => github.com/openshift/kubernetes/staging/src/k8s.io/component-base v0.0.0-20221020223141-4bd0702b5791
	k8s.io/component-helpers => github.com/openshift/kubernetes/staging/src/k8s.io/component-helpers v0.0.0-20221020223141-4bd0702b5791
	k8s.io/controller-manager => github.com/openshift/kubernetes/staging/src/k8s.io/controller-manager v0.0.0-20221020223141-4bd0702b5791
	k8s.io/cri-api => github.com/openshift/kubernetes/staging/src/k8s.io/cri-api v0.0.0-20221020223141-4bd0702b5791
	k8s.io/csi-translation-lib => github.com/openshift/kubernetes/staging/src/k8s.io/csi-translation-lib v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kube-aggregator => github.com/openshift/kubernetes/staging/src/k8s.io/kube-aggregator v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kube-controller-manager => github.com/openshift/kubernetes/staging/src/k8s.io/kube-controller-manager v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kube-proxy => github.com/openshift/kubernetes/staging/src/k8s.io/kube-proxy v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kube-scheduler => github.com/openshift/kubernetes/staging/src/k8s.io/kube-scheduler v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kubectl => github.com/openshift/kubernetes/staging/src/k8s.io/kubectl v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kubelet => github.com/openshift/kubernetes/staging/src/k8s.io/kubelet v0.0.0-20221020223141-4bd0702b5791
	k8s.io/kubernetes => github.com/openshift/kubernetes v1.25.1-0.20221020223141-4bd0702b5791
	k8s.io/legacy-cloud-providers => github.com/openshift/kubernetes/staging/src/k8s.io/legacy-cloud-providers v0.0.0-20221020223141-4bd0702b5791
	k8s.io/metrics => github.com/openshift/kubernetes/staging/src/k8s.io/metrics v0.0.0-20221020223141-4bd0702b5791
	k8s.io/mount-utils => github.com/openshift/kubernetes/staging/src/k8s.io/mount-utils v0.0.0-20221020223141-4bd0702b5791
	k8s.io/pod-security-admission => github.com/openshift/kubernetes/staging/src/k8s.io/pod-security-admission v0.0.0-20221020223141-4bd0702b5791
	k8s.io/sample-apiserver => github.com/openshift/kubernetes/staging/src/k8s.io/sample-apiserver v0.0.0-20221020223141-4bd0702b5791
	k8s.io/sample-cli-plugin => github.com/openshift/kubernetes/staging/src/k8s.io/sample-cli-plugin v0.0.0-20221020223141-4bd0702b5791
	k8s.io/sample-controller => github.com/openshift/kubernetes/staging/src/k8s.io/sample-controller v0.0.0-20221020223141-4bd0702b5791
)
