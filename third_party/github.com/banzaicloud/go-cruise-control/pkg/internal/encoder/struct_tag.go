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

package encoder

import (
	"strings"

	"github.com/pkg/errors"
)

const (
	StructTagKey = "param"
	// StructTagDelimiter used to separate items in struct tags.
	StructTagDelimiter = ","
	// StructTagSeparator used for defining key/value flags in struct tags.
	StructTagSeparator = "="
	// StructTagFlagOmitEmpty used to determine whether the field can be omitted if it's Value is empty or not.
	StructTagFlagOmitEmpty = "omitempty"
)

// StructTag stores information about the struct tags which includes the name of the Key and optional flags like
// `omitempty`.
type StructTag struct {
	// Key name which is the first item of a struct tag
	Key string
	// Flags
	OmitEmpty bool
}

// Skip returns true if the Key field of the structTag is either an empty string or `-` meaning
// that the struct field should not be considered during marshal/unmarshal.
func (t StructTag) Skip() bool {
	return t.Key == "" || t.Key == "-"
}

// StructTagFlag holds information about flags defined for a struct tag in a Key/Value format.
type StructTagFlag struct {
	Key   string
	Value string
}

// IsValid considers f StructTagFlag valid if Key field is a non-empty string.
func (f StructTagFlag) IsValid() bool {
	return f.Key != ""
}

// ParseStructTagFlag returns a pointer to a StructTagFlag object holding the information resulted from parsing the
// s string as a struct tag flag.
func ParseStructTagFlag(s string) (*StructTagFlag, error) {
	if s == "" {
		return nil, errors.New("struct tag flag must not be empty string")
	}
	flag := StructTagFlag{}
	f := strings.SplitN(s, StructTagSeparator, 2) //nolint:gomnd
	flag.Key = f[0]
	// There might be flags with values like "flag=value"
	if len(f) == 2 { //nolint:gomnd
		flag.Value = f[1]
	}
	return &flag, nil
}

// ParseStructTag returns a pointer to a StructTag object holding the information resulted from parsing the
// s string as a struct tag.
func ParseStructTag(s string) (*StructTag, error) {
	if s == "" {
		return nil, errors.New("struct tag must not be empty string")
	}
	st := &StructTag{}
	// Split struct tag by delimiter character
	items := strings.Split(s, StructTagDelimiter)
	st.Key = items[0]
	// Parse flags if they are present in the struct tag
	if len(items) > 1 {
		// Iterate over the flags
		for _, f := range items[1:] {
			// Parse struct tag flag
			flag, err := ParseStructTagFlag(f)
			if err != nil {
				return nil, err
			}
			// Handle supported flags
			switch flag.Key {
			case StructTagFlagOmitEmpty:
				st.OmitEmpty = true
			default:
				return nil, errors.Errorf("struct tag flag is not supported: %s", f)
			}
		}
	}
	return st, nil
}
