// Copyright 2025 Dmitrii Okunev.
// Copyright 2023 Meta Platforms, Inc. and affiliates.
//
// Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its contributors may be used to endorse or promote products derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package polyjson

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// TypeID is an unique identifier of a type
type TypeID string

// TypeIDOfer is a converter of a sample to its TypeID.
type TypeIDOfer interface {
	// TypeIDOf returns TypeID of the type of the given sample.
	TypeIDOf(sample any) (TypeID, error)
}

// NewByTypeIDer is a factory of a value given its TypeID.
type NewByTypeIDer interface {
	// NewByTypeID returns a pointer to an object of the type specified through TypeID.
	NewByTypeID(TypeID) (any, error)
}

// TypeIDHandler is a bidirectional handler which couples TypeID with a type.
type TypeIDHandler interface {
	TypeIDOfer
	NewByTypeIDer
}

// MarshalWithTypeIDs is similar to json.Marshal, but any interface field
// met in a structure is serialized as a structure containing the type
// identifier and the value. It allows to unmarshal the result without
// loosing typing.
//
// If an interface is met, then instead of marshaling its content directly,
// we resolve its type ID through TypeIDOfer and putting:
//
//	{ResolvedTypeID: {...Content...}}
//
// instead (where ResolvedTypeID is a string containing the TypeID).
//
// For example:
//
//	type Struct {
//	    Field any
//	}
//	xjson.MarshalWithTypeIDs(Struct{Field: Struct{Field: int(1)}}, typeIDOfer)
//
// might be marshalled to
//
//	{"Field": {"Struct": {"Field": {"int": 1}}}}
//
// NOTE! This is not a drop-in replacement for standard json.Marshal.
//
//	It has incompatible behavior.
func MarshalWithTypeIDs(obj any, typeIDOfer TypeIDOfer) ([]byte, error) {
	return marshal(reflect.ValueOf(obj), typeIDOfer)
}

var stringNull = []byte("null")

func marshal(v reflect.Value, typeIDOfer TypeIDOfer) ([]byte, error) {
	// How the function works:
	//
	// We are interested only about structures (and their fields),
	// everything else is handled by standard json.Marshal
	//
	// We just iterate through fields and add TypeIDs if see an interface,
	// otherwise marshal as is.

	switch v.Kind() {
	case reflect.Interface:
		// unwrapping the interface
		v := reflect.ValueOf(v.Interface())
		if !v.IsValid() {
			// there was the untyped nil value behind the interface
			return stringNull, nil
		}
		return marshal(v, typeIDOfer)
	case reflect.Pointer:
		v := v.Elem()
		if !v.IsValid() {
			// is a nil pointer
			return stringNull, nil
		}
		// A pointer may lead to a structure, dereferencing and going deeper.
		return marshal(v, typeIDOfer)
	case reflect.Map:
		// marshaledFields contains the map of JSON field name to marshalled valued
		marshaledFields := map[string]any{}
		iterator := v.MapRange()
		for iterator.Next() {
			key := iterator.Key()
			value := iterator.Value()

			// Constructing the field name

			jsonFieldName, err := stringifyMapKey(key)
			if err != nil {
				return nil, fmt.Errorf("unable to stringify map key of type %T: %w", key.Interface(), err)
			}

			// Marshalling the content

			b, err := marshal(value, typeIDOfer)
			if err != nil {
				return nil, fmt.Errorf("unable to serialize value of map-entry with key '%s': %w", jsonFieldName, err)
			}

			// TODO: deduplicate the code below with the same code in the reflect.Struct case
			// If the field is not interface, then putting the content directly
			if v.Type().Elem().Kind() != reflect.Interface || !reflect.ValueOf(value.Interface()).IsValid() {
				marshaledFields[jsonFieldName] = b
				continue
			}

			// If the field is an interface, then put the value in format: {TypeID: {..Content..}}

			typeID, err := typeIDOfer.TypeIDOf(value.Interface())
			if err != nil {
				return nil, fmt.Errorf("unable to get TypeID of %T: %w", value.Interface(), err)
			}
			marshaledFields[jsonFieldName] = map[TypeID]json.RawMessage{
				typeID: json.RawMessage(b),
			}
		}
		return json.Marshal(marshaledFields)
	case reflect.Slice, reflect.Array:
		// conversion for slices and arrays is not supported, yet
		return json.Marshal(v.Interface())
	case reflect.Struct:
		t := v.Type()

		// marshaledFields contains the map of JSON field name to marshalled valued
		marshaledFields := map[string]any{}

		// Iterating through structure fields:
		for i := 0; i < v.NumField(); i++ {
			fT := t.Field(i)
			fV := v.Field(i)

			if fT.PkgPath != "" {
				// unexported
				continue
			}

			// Detecting the field name

			tag := fT.Tag.Get("json")
			if tag == "-" {
				// requested to skip
				continue
			}
			tagWords := strings.Split(tag, ",")

			jsonFieldName := fT.Name
			if len(tagWords[0]) > 0 {
				jsonFieldName = tagWords[0]
			}

			// Marshalling the content

			b, err := marshal(fV, typeIDOfer)
			if err != nil {
				return nil, fmt.Errorf("unable to serialize data within field #%d:%s of structure %T: %w", i, fT.Name, v.Interface(), err)
			}

			// If the field is not interface or it is an untyped nil, then putting the content directly
			if fT.Type.Kind() != reflect.Interface || !reflect.ValueOf(fV.Interface()).IsValid() {
				marshaledFields[jsonFieldName] = json.RawMessage(b)
				continue
			}

			// If the field is an interface, then put the value in format: {TypeID: {..Content..}}

			typeID, err := typeIDOfer.TypeIDOf(fV.Interface())
			if err != nil {
				return nil, fmt.Errorf("unable to get TypeID of %T: %w", fV.Interface(), err)
			}
			marshaledFields[jsonFieldName] = map[TypeID]json.RawMessage{
				typeID: json.RawMessage(b),
			}
		}

		// Now we get the map of JSON field names to JSONized values. Just compiling this into the final JSON:
		return json.Marshal(marshaledFields)
	}

	// Everything else:
	return json.Marshal(v.Interface())
}

func stringifyMapKey(mapKey reflect.Value) (string, error) {
	if mapKey.Kind() == reflect.String {
		return mapKey.String(), nil
	}

	return "", fmt.Errorf("unable to stringify map key '%#+v' (%T)", mapKey.Interface(), mapKey.Interface())
}
