module github.com/banzaicloud/koperator

go 1.16

require (
	emperror.dev/errors v0.8.0
	github.com/Shopify/sarama v1.29.1
	github.com/banzaicloud/bank-vaults/pkg/sdk v0.7.0
	github.com/banzaicloud/istio-client-go v0.0.10
	github.com/banzaicloud/istio-operator/pkg/apis v0.10.6
	github.com/banzaicloud/k8s-objectmatcher v1.5.2
	github.com/banzaicloud/koperator/api v0.0.0
	github.com/banzaicloud/koperator/properties v0.1.0
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cncf/xds/go v0.0.0-20211007041622-c0841ac0dd72 // indirect
	github.com/containerd/containerd v1.5.7 // indirect
	github.com/envoyproxy/go-control-plane v0.9.9
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-logr/logr v0.4.0
	github.com/hashicorp/go-retryablehttp v0.7.0 // indirect
	github.com/hashicorp/vault v1.8.4
	github.com/hashicorp/vault/api v1.1.2-0.20210713235431-1fc8af4c041f
	github.com/hashicorp/vault/sdk v0.2.2-0.20211005222123-93e045565e4a
	github.com/imdario/mergo v0.3.12
	github.com/influxdata/influxdb v1.9.3 // indirect
	github.com/jetstack/cert-manager v1.5.4
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/lestrrat-go/backoff v1.0.1
	github.com/mitchellh/mapstructure v1.4.2 // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/pavel-v-chernykh/keystore-go/v4 v4.1.0
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/prometheus/common v0.30.1
	github.com/shirou/gopsutil v3.21.8+incompatible // indirect
	github.com/tencentcloud/tencentcloud-sdk-go v3.0.171+incompatible // indirect
	github.com/tklauser/go-sysconf v0.3.9 // indirect
	go.uber.org/zap v1.19.1
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519 // indirect
	golang.org/x/oauth2 v0.0.0-20211005180243-6b3c2da341f1 // indirect
	golang.org/x/sys v0.0.0-20211007075335-d3039528d8ac // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	google.golang.org/genproto v0.0.0-20211007155348-82e027067bd4 // indirect
	google.golang.org/protobuf v1.27.1
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/kube-openapi v0.0.0-20210929172449-94abcedd1aa4 // indirect
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b // indirect
	sigs.k8s.io/controller-runtime v0.9.7
)

replace (
	github.com/banzaicloud/koperator/api => ./api
	github.com/banzaicloud/koperator/properties => ./properties
)
