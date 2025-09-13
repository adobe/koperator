/*
Copyright © 2021 Cisco and/or its affiliates. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	DefaultServerURL = "http://localhost:8090/kafkacruisecontrol"
	DefaultUserAgent = "go-cruise-control"
)

type Client struct {
	httpClient *http.Client
	url        *url.URL
	auth       AuthInfo
	userAgent  string
}

func (c Client) String() string {
	return fmt.Sprintf("CruiseControlClient\n\turl: %s\n\tuseragent: %s\n", c.url, c.userAgent)
}

func (c Client) send(ctx context.Context, req *http.Request, opts ...RequestOptions) (*http.Response, error) {
	log := logr.FromContextOrDiscard(ctx)

	opts = append(opts, []RequestOptions{
		WithServerURL(c.url),
		WithAuthInfo(c.auth),
		WithUserAgent(c.userAgent),
		WithAcceptJSON(),
		WithContentTypeJSON(),
		WithJSONQuery(),
	}...)

	for _, o := range opts {
		if err := o.apply(req); err != nil {
			return nil, fmt.Errorf("failed to apply option(s) to HTTP request: %w", err)
		}
	}
	log.V(-1).Info("sending request", "url", req.URL, "method", req.Method)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("sending HTTP request failed: %w", err)
	}
	return resp, err
}

func (c Client) request(ctx context.Context, req interface{}, resp types.APIResponse, e types.APIEndpoint, m string) error {
	log := logr.FromContextOrDiscard(ctx)

	r, err := MarshalRequest(req)
	if err != nil {
		return err
	}

	opts := []RequestOptions{
		WithEndpoint(e),
		WithMethod(m),
		WithContext(ctx),
	}

	if _, ok := req.(types.RequestReasoner); ok {
		opts = append(opts, WithReasonFromContext(ctx))
	}

	httpResp, err := c.send(ctx, r, opts...)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err = Body.Close()
		if err != nil {
			log.V(-1).Info("failed to close response body for request", "url", httpResp.Request.URL)
		}
	}(httpResp.Body)

	log.V(0).Info("got response for request", "url", httpResp.Request.URL,
		"status", httpResp.StatusCode)

	contentType := parseContentType(httpResp.Header.Get(HTTPHeaderContentType))
	if contentType.MIMEType != MIMETypeJSON && contentType.ChartSet != ChartSetUTF8 {
		return errors.Errorf("content type mismatch for request %s: expected %s; %s, got %s; %s", httpResp.Request.URL,
			MIMETypeJSON, ChartSetUTF8, contentType.MIMEType, contentType.ChartSet)
	}

	if err = resp.UnmarshalResponse(httpResp); err != nil {
		return fmt.Errorf("failed to convert HTTP response to API response: %w", err)
	}

	if resp.Failed() {
		return fmt.Errorf("HTTP request failed: %w", resp.Err())
	}
	return nil
}

// NewClient returns a new API client with the provided configuration. It returns an error if the configuration
// is invalid.
func NewClient(opts *Config) (*Client, error) {
	var err error

	client := &Client{}

	// NOTE: user task caching does not work properly on Cruise Control end as it returns stalled responses.
	// jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	// if err != nil {
	// 	return nil, err
	// }
	// client.httpClient = &http.Client{
	// 	Transport: http.DefaultTransport,
	// 	Jar:       jar,
	// 	Timeout:   0,
	// }

	client.httpClient = opts.HTTPClient
	if client.httpClient == nil {
		client.httpClient = &http.Client{
			Transport: http.DefaultTransport,
		}
	}

	serverURL := opts.ServerURL
	if serverURL == "" {
		serverURL = DefaultServerURL
	}
	if !strings.HasSuffix(serverURL, "/") {
		serverURL += "/"
	}
	if client.url, err = url.Parse(serverURL); err != nil {
		return nil, fmt.Errorf("failed to parse Cruise Control server URL: %w", err)
	}

	switch opts.AuthType {
	case AuthTypeBasic:
		client.auth = &BasicAuth{
			username: opts.Username,
			password: opts.Password,
		}
	case AuthTypeAccessToken:
		client.auth = &AccessTokenAuth{
			token: opts.AccessToken,
		}
	case AuthTypeNone:
		fallthrough
	default:
		client.auth = nil
	}

	client.userAgent = opts.UserAgent
	if client.userAgent == "" {
		client.userAgent = DefaultUserAgent
	}

	return client, nil
}

// NewDefaultClient return a new API client with default configuration. It returns an error if the configuration
// is invalid.
func NewDefaultClient() (*Client, error) {
	return NewClient(&Config{
		ServerURL: DefaultServerURL,
		AuthType:  AuthTypeNone,
		UserAgent: DefaultUserAgent,
	})
}
