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
	"reflect"
	"strings"

	"github.com/pkg/errors"

	"github.com/banzaicloud/go-cruise-control/pkg/internal/encoder"
)

const reasonContextKey reasonContextKeyType = "CruiseControlRequestReason"

type reasonContextKeyType string

// RequestMarshaler is the interface implemented by types that can marshal themselves into valid http.Request
type RequestMarshaler interface {
	MarshalRequest() (*http.Request, error)
}

func MarshalRequest(v interface{}) (*http.Request, error) {
	marshalerType := reflect.TypeOf((*RequestMarshaler)(nil)).Elem()

	// Get the Value of the v interface
	vValue := reflect.Indirect(reflect.ValueOf(v))

	if !vValue.IsValid() {
		return nil, errors.New("request: cannot marshal invalid value")
	}

	if vValue.IsZero() {
		return &http.Request{URL: &url.URL{}}, nil
	}

	// Get the type of the interface
	vType := vValue.Type()

	// Check if the v implements the RequestMarshaler and call it's custom marshaling logic if so.
	if vType.Implements(marshalerType) {
		u, ok := vValue.Interface().(RequestMarshaler)
		if !ok {
			return nil, errors.Errorf("request: could not cast to %s", marshalerType)
		}

		req, err := u.MarshalRequest()
		if err != nil {
			err = fmt.Errorf("failed to encode API request to HTTP request: %w", err)
		}
		return req, err
	}

	return marshal(v)
}

func marshal(v interface{}) (*http.Request, error) {
	p, err := encoder.MarshalParams(v)
	if err != nil {
		return nil, fmt.Errorf("failed to encode API request to HTTP query parameters: %w", err)
	}

	for pKey, pValue := range p {
		if len(pValue) > 1 {
			p.Set(pKey, strings.Join(pValue, ","))
		}
	}

	r := &http.Request{
		URL: &url.URL{
			RawQuery: url.Values(p).Encode(),
		},
	}

	return r, nil
}

func ContextWithReason(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, reasonContextKey, reason)
}

func ReasonFromContext(ctx context.Context) (string, bool) {
	if r := ctx.Value(reasonContextKey); r != nil {
		if rr, ok := r.(string); ok {
			return rr, true
		}
	}
	return "", false
}
