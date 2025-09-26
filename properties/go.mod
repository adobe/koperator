module github.com/banzaicloud/koperator/properties

go 1.25

require (
	emperror.dev/errors v0.8.1
	github.com/onsi/gomega v1.38.2
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.44.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)

// remove once https://github.com/cert-manager/cert-manager/issues/5953 is fixed
replace github.com/Venafi/vcert/v4 => github.com/jetstack/vcert/v4 v4.9.6-0.20230519122548-219f317ae107
