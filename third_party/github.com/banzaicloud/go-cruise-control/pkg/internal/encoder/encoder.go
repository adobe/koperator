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
	"fmt"
	"reflect"

	"github.com/pkg/errors"

	"github.com/banzaicloud/go-cruise-control/pkg/types"
)

const (
	defaultMaxRecursion = 5
	rootParamKey        = "_"
)

type ParamsMarshaler interface {
	MarshalParams(key string) (types.Params, error)
}

type encoderState struct {
	params types.Params

	recursion uint
	ptrSeen   map[uintptr]bool
}

func (s *encoderState) next() (*encoderState, error) {
	if s.recursion >= 1 {
		return &encoderState{
			recursion: s.recursion - 1,
			ptrSeen:   s.ptrSeen,
			params:    s.params,
		}, nil
	}
	return nil, errors.New("encoder: reached max recursion")
}

func (s *encoderState) reset() {
	s.params = make(types.Params)
}

func (s encoderState) String() string {
	ptrs := make([]uintptr, 0)
	for p := range s.ptrSeen {
		ptrs = append(ptrs, p)
	}
	return fmt.Sprintf("params: %s\nrecursion: %d\npointers seen: %v\n", s.params, s.recursion, ptrs)
}

func newEncoderState(maxRecursion uint) *encoderState {
	return &encoderState{
		params:    make(types.Params),
		recursion: maxRecursion,
		ptrSeen:   make(map[uintptr]bool),
	}
}

type encoderOptions struct {
	ParamKey  string
	OmitEmpty bool
}

func (o encoderOptions) AtRoot() bool {
	return o.ParamKey == rootParamKey
}

func newEncoderOptions() encoderOptions {
	return encoderOptions{
		ParamKey:  rootParamKey,
		OmitEmpty: true,
	}
}

type encoderFunc func(s *encoderState, v reflect.Value, o encoderOptions) error

func MarshalParams(v interface{}) (types.Params, error) {
	state := newEncoderState(defaultMaxRecursion)
	encoder := newTypeEncoder(reflect.TypeOf(v))
	if err := encoder(state, reflect.ValueOf(v), newEncoderOptions()); err != nil {
		return nil, err
	}
	return state.params, nil
}

func newTypeEncoder(t reflect.Type) encoderFunc {
	paramsMarshalerType := reflect.TypeOf((*ParamsMarshaler)(nil)).Elem()

	if t.Implements(paramsMarshalerType) {
		return marshalerEncoder
	}

	switch t.Kind() { //nolint:exhaustive
	case reflect.Bool:
		return stringEncoder
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return stringEncoder
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return stringEncoder
	case reflect.Float32:
		return stringEncoder
	case reflect.Float64:
		return stringEncoder
	case reflect.String:
		return stringEncoder
	case reflect.Interface:
		return interfaceEncoder
	case reflect.Struct:
		return structEncoder
	case reflect.Map:
		return newMapEncoder(t)
	case reflect.Slice:
		return newSliceEncoder(t)
	case reflect.Array:
		return newArrayEncoder(t)
	case reflect.Ptr:
		return newPtrEncoder(t)
	default:
		return unsupportedTypeEncoder
	}
}

func marshalerEncoder(s *encoderState, v reflect.Value, o encoderOptions) error {
	paramsMarshalerType := reflect.TypeOf((*ParamsMarshaler)(nil)).Elem()

	if v.Kind() == reflect.Ptr && v.IsNil() {
		if o.OmitEmpty {
			return nil
		}
		return errors.Errorf("params: value for required key %s is empty pointer", o.ParamKey)
	}
	m, ok := v.Interface().(ParamsMarshaler)
	if !ok {
		return errors.Errorf("params: casting type for key %s to %s interface failed", o.ParamKey, paramsMarshalerType)
	}
	p, err := m.MarshalParams(o.ParamKey)
	if err != nil {
		return errors.Errorf("params: marshaling data for key %s has failed", o.ParamKey)
	}
	s.params.Merge(p)
	return nil
}

func stringEncoder(s *encoderState, v reflect.Value, o encoderOptions) error {
	if v.IsZero() && o.OmitEmpty {
		return nil
	}
	s.params.Add(o.ParamKey, fmt.Sprintf("%v", reflect.Indirect(v)))
	return nil
}

func structEncoder(s *encoderState, v reflect.Value, o encoderOptions) error {
	ns, err := s.next()
	if err != nil {
		return err
	}

	vValue := reflect.Indirect(v)
	if !vValue.IsValid() {
		return nil
	}
	vType := vValue.Type()
	numFields := vType.NumField()

	for i := 0; i < numFields; i++ {
		vField := vType.Field(i)
		st := &StructTag{
			Key:       rootParamKey,
			OmitEmpty: true,
		}

		// Check if StructTagKey key is present for this struct field and move on to the next field if not.
		if tag, found := vField.Tag.Lookup(StructTagKey); found {
			// Parse struct tag and move on to the next field if the tag name is empty.
			st, err = ParseStructTag(tag)
			if err != nil {
				return err
			}
			if st.Skip() {
				continue
			}
		}

		opts := encoderOptions{
			ParamKey:  st.Key,
			OmitEmpty: st.OmitEmpty,
		}

		if err = newTypeEncoder(vField.Type)(ns, vValue.Field(i), opts); err != nil {
			return err
		}
	}
	return nil
}

type ptrEncoder struct {
	elemEnc encoderFunc
}

func (e ptrEncoder) encode(s *encoderState, v reflect.Value, o encoderOptions) error {
	if _, ok := s.ptrSeen[v.Pointer()]; !ok && !v.IsNil() {
		return e.elemEnc(s, v, o)
	}
	return nil
}

func newPtrEncoder(t reflect.Type) encoderFunc {
	enc := ptrEncoder{newTypeEncoder(t.Elem())}
	return enc.encode
}

type arrayEncoder struct {
	elemEnc encoderFunc
}

func (e arrayEncoder) encode(s *encoderState, v reflect.Value, o encoderOptions) error {
	ne, err := s.next()
	if err != nil {
		return err
	}
	ne.reset()

	l := v.Len()
	for i := 0; i < l; i++ {
		err = e.elemEnc(ne, v.Index(i), o)
		if err != nil {
			return err
		}
	}
	s.params.Merge(ne.params)
	return nil
}

func newArrayEncoder(t reflect.Type) encoderFunc {
	enc := arrayEncoder{newTypeEncoder(t.Elem())}
	return enc.encode
}

func newSliceEncoder(t reflect.Type) encoderFunc {
	paramsMarshalerType := reflect.TypeOf((*ParamsMarshaler)(nil)).Elem()
	stringerType := reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

	if t.Elem().Kind() == reflect.Uint8 {
		p := reflect.PtrTo(t.Elem())
		if !p.Implements(paramsMarshalerType) && !p.Implements(stringerType) {
			return byteSliceEncoder
		}
	}
	enc := sliceEncoder{newArrayEncoder(t)}
	return enc.encode
}

func byteSliceEncoder(s *encoderState, v reflect.Value, o encoderOptions) error {
	if !v.IsNil() {
		s.params.Add(o.ParamKey, string(v.Bytes()))
	}
	return nil
}

type sliceEncoder struct {
	elemEnc encoderFunc
}

func (e *sliceEncoder) encode(s *encoderState, v reflect.Value, o encoderOptions) error {
	if _, ok := s.ptrSeen[v.Pointer()]; !ok && !v.IsNil() {
		ns, err := s.next()
		if err != nil {
			return err
		}

		err = e.elemEnc(ns, v, o)
		if err != nil {
			return err
		}
	}
	return nil
}

func newMapEncoder(t reflect.Type) encoderFunc {
	stringerType := reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

	switch t.Key().Kind() { //nolint:exhaustive
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
	default:
		if !t.Key().Implements(stringerType) {
			return unsupportedTypeEncoder
		}
	}
	enc := mapEncoder{newTypeEncoder(t.Elem())}
	return enc.encode
}

type mapEncoder struct {
	elemEnc encoderFunc
}

func (e *mapEncoder) encode(s *encoderState, v reflect.Value, o encoderOptions) error {
	ns, err := s.next()
	if err != nil {
		return err
	}
	ns.reset()

	m := v.MapRange()
	for m.Next() {
		mk := m.Key()
		mv := m.Value()

		opts := encoderOptions{
			ParamKey:  fmt.Sprintf("%v", reflect.Indirect(mk)),
			OmitEmpty: o.OmitEmpty,
		}

		err = e.elemEnc(ns, mv, opts)
		if err != nil {
			return err
		}
	}
	if o.AtRoot() {
		s.params.Merge(ns.params)
	} else {
		s.params.Add(o.ParamKey, ns.params.List()...)
	}
	return nil
}

func interfaceEncoder(s *encoderState, v reflect.Value, o encoderOptions) error {
	if v.IsNil() {
		return nil
	}
	return newTypeEncoder(v.Elem().Type())(s, v, o)
}

func unsupportedTypeEncoder(_ *encoderState, v reflect.Value, _ encoderOptions) error {
	return errors.Errorf("params: unsupported type %s", v.Type().Kind())
}
