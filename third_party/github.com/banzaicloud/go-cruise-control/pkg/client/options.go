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
	"net/http"
	"net/url"
	"path"

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	HTTPHeaderUserAgent   = "User-Agent"
	HTTPHeaderAccept      = "Accept"
	HTTPHeaderContentType = "Content-Type"
	MIMETypeJSON          = "application/json"
	ChartSetUTF8          = "utf-8"
	JSONQueryParam        = "json"
	ReasonQueryParam      = "reason"
)

type RequestOptions interface {
	apply(*http.Request) error
}

type RequestOptionApplier func(*http.Request) error

func (c RequestOptionApplier) apply(r *http.Request) error {
	return c(r)
}

func WithAuthInfo(a AuthInfo) RequestOptionApplier {
	return func(r *http.Request) error {
		if a != nil {
			err := a.Apply(r)
			if err != nil {
				return fmt.Errorf("failed to set authentication information to HTTP request: %w", err)
			}
			return nil
		}
		return nil
	}
}

func WithServerURL(u *url.URL) RequestOptionApplier {
	return func(r *http.Request) error {
		if r.URL == nil {
			r.URL = &url.URL{}
		}
		baseURL := *u
		reqURL := baseURL.ResolveReference(r.URL)
		r.URL = reqURL
		return nil
	}
}

func WithEndpoint(endpoint types.APIEndpoint) RequestOptionApplier {
	return func(r *http.Request) error {
		if r.URL == nil {
			r.URL = &url.URL{}
		}
		r.URL.Path = path.Join(r.URL.Path, endpoint.Path())
		return nil
	}
}

func WithMethod(m string) RequestOptionApplier {
	return func(r *http.Request) error {
		r.Method = m
		return nil
	}
}

func WithHeader(h string, v string) RequestOptionApplier {
	return func(r *http.Request) error {
		if r.Header == nil {
			r.Header = make(http.Header)
		}
		r.Header.Set(h, v)
		return nil
	}
}

func WithUserAgent(agent string) RequestOptionApplier {
	return WithHeader(HTTPHeaderUserAgent, agent)
}

func WithAcceptJSON() RequestOptionApplier {
	return WithHeader(HTTPHeaderAccept, MIMETypeJSON)
}

func WithContentTypeJSON() RequestOptionApplier {
	return WithHeader(HTTPHeaderContentType, fmt.Sprintf("%s; charset=%s", MIMETypeJSON, ChartSetUTF8))
}

func WithJSONQuery() RequestOptionApplier {
	return WithQuery(JSONQueryParam, "true")
}

func WithContext(ctx context.Context) RequestOptionApplier {
	return func(r *http.Request) error {
		r2 := r.WithContext(ctx)
		*r = *r2
		return nil
	}
}

func WithReasonFromContext(ctx context.Context) RequestOptionApplier {
	reason, ok := ReasonFromContext(ctx)
	if !ok {
		return func(r *http.Request) error { return nil }
	}
	return WithQuery(ReasonQueryParam, reason)
}

func WithQuery(k, v string) RequestOptionApplier {
	return func(r *http.Request) error {
		if r.URL == nil {
			r.URL = &url.URL{}
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			return fmt.Errorf("failed to apply %s query parameter to HTTP request: %w", k, err)
		}
		q.Set(k, v)
		r.URL.RawQuery = q.Encode()
		return nil
	}
}
