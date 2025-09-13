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
	"fmt"
	"net/http"
)

const (
	HTTPHeaderAuthorization = "Authorization"
)

type AuthInfo interface {
	Apply(r *http.Request) error
}

type BasicAuth struct {
	username string
	password string
}

func (a BasicAuth) Apply(r *http.Request) error {
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.SetBasicAuth(a.username, a.password)
	return nil
}

type AccessTokenAuth struct {
	token string
}

func (a AccessTokenAuth) Apply(r *http.Request) error {
	if r.Header == nil {
		r.Header = make(http.Header)
	}
	r.Header.Set(HTTPHeaderAuthorization, fmt.Sprintf("Bearer %s", a.token))
	return nil
}

type AuthType int8

func (t AuthType) String() string {
	switch t {
	case AuthTypeBasic:
		return "BASIC"
	case AuthTypeAccessToken:
		return "ACCESS_TOKEN"
	case AuthTypeNone:
		fallthrough
	default:
		return "NONE"
	}
}

func AuthTypeFromString(s string) AuthType {
	switch s {
	case AuthTypeBasic.String():
		return AuthTypeBasic
	case AuthTypeAccessToken.String():
		return AuthTypeAccessToken
	default:
		return AuthTypeNone
	}
}

const (
	AuthTypeNone AuthType = iota
	AuthTypeBasic
	AuthTypeAccessToken
)
