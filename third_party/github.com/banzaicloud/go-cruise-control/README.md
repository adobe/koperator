# go-cruise-control

[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/banzaicloud/go-cruise-control/ci.yaml?style=flat-square)](https://github.com/banzaicloud/go-cruise-control/actions/workflows/ci.yaml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/banzaicloud/go-cruise-control/badge?style=flat-square)](https://api.securityscorecards.dev/projects/github.com/banzaicloud/go-cruise-control)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/banzaicloud/go-cruise-control?style=flat-square)](https://github.com/banzaicloud/go-cruise-control/releases/latest)

It's client library (written in Golang) for interacting with
[Linkedin Cruise Control](https://github.com/linkedin/cruise-control) using its HTTP API.

Supported _Cruise Control_ versions: **2.5.94+** (tested with v2.5.101)

## How to use it

```shell
go get github.com/banzaicloud/go-cruise-control@latest
```

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/banzaicloud/go-cruise-control/client"
	"github.com/banzaicloud/go-cruise-control/api"
)

func main() {
	// Initializing Cruise Control client with default configuration
	cruisecontrol, err := client.NewDefaultClient()
	if err != nil {
		panic(err)
	}

	// Create Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30 * time.Second)
	defer cancel() 

	// Optionally set request Reason to Context which will sent to Cruise Control as part of the HTTP request
	ctx = client.ContextWithReason("example")

	// Assembling the request using default values
	req := api.StateRequestWithDefaults()

	// Sending the request to the State API
	resp, err := cruisecontrol.State(ctx, req)
	if err != nil {
		panic(err)
	}

	// Getting the state of the Executor
	fmt.Println(resp.Result.ExecutorState.State)
}
```

## Development

### Prerequisites

* golang `1.18`
* docker
* docker-compose

### Development environment

Starting local development environment

```shell
make start
```

Tearing down the local development environment

```shell
make stop
```

### Testing

Running integration tests creates a new test environment prior executing the test suite and tears down at the end.

```shell
make integration-test
```

Set the `USE_EXISTING=true` environment variable if reusing the existing development/test environment
for integration testing is the desired behaviour.

```shell
export USE_EXISTING=true
make integration-test
```

## Contributing

If you find this project useful, help us:

* Support the development of this project and star this repo! ⭐
* Help new users with issues they may encounter 💪
* Send a pull request with your new features and bug fixes 🚀

## Community

Find us on [Slack](https://outshift.slack.com/channels/koperator)!

<a href="https://github.com/banzaicloud/go-cruise-control/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=banzaicloud/koperator&max=30" />
</a>

## License

Copyright (c) 2023 [Cisco Systems, Inc.](https://www.cisco.com) and/or its affiliates

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
