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
	"net/http"
	"os"
)

const (
	prefix            = "CC_"
	ServerURLEnvKey   = prefix + "SERVER_URL"
	AuthTypeEnvKey    = prefix + "AUTH_TYPE"
	UsernameEnvKey    = prefix + "USERNAME"
	PasswordEnvKey    = prefix + "PASSWORD"
	AccessTokenEnvKey = prefix + "ACCESS_TOKEN"
	UserAgentEnvKey   = prefix + "USER_AGENT"
)

// Config contains the configuration parameters for the API Client
type Config struct {
	ServerURL   string
	AuthType    AuthType
	Username    string
	Password    string
	AccessToken string
	UserAgent   string

	HTTPClient *http.Client
}

func (c *Config) ReadFromEnvironment() {
	c.ServerURL = os.Getenv(ServerURLEnvKey)
	c.AuthType = AuthTypeFromString(os.Getenv(AuthTypeEnvKey))
	c.Username = os.Getenv(UsernameEnvKey)
	c.Password = os.Getenv(PasswordEnvKey)
	c.AccessToken = os.Getenv(AccessTokenEnvKey)
	c.UserAgent = os.Getenv(UserAgentEnvKey)
}
