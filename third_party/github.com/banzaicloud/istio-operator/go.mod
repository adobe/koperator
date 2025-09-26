module github.com/banzaicloud/istio-operator/v2

go 1.25

replace github.com/banzaicloud/istio-operator/api/v2 => ./api

// needs a fork to support istio operator v2 api int64/uint64 marshalling to integers
replace github.com/golang/protobuf => github.com/luciferinlove/protobuf v1.5.2
