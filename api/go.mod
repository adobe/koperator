module github.com/banzaicloud/koperator/api

go 1.21

require (
	dario.cat/mergo v1.0.0
	emperror.dev/errors v0.8.1
	github.com/banzaicloud/istio-client-go v0.0.17
	github.com/cert-manager/cert-manager v1.14.4
	golang.org/x/exp v0.0.0-20240404231335-c0f41cb1a7a0
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.29.3
	k8s.io/apimachinery v0.29.3
	sigs.k8s.io/controller-runtime v0.17.2
)

require (
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/utils v0.0.0-20240310230437-4693a0247e57 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
)

// remove once https://github.com/cert-manager/cert-manager/issues/5953 is fixed
replace github.com/Venafi/vcert/v4 => github.com/jetstack/vcert/v4 v4.9.6-0.20230127103832-3aa3dfd6613d

replace github.com/imdario/mergo => github.com/imdario/mergo v0.3.16
