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

	"github.com/tidwall/gjson"
)

func unstringifyMapKey(mapKey reflect.Value, s string) error {
	if mapKey.Kind() == reflect.String {
		mapKey.SetString(s)
		return nil
	}

	return fmt.Errorf("unable to unstringify map key (%T) value '%s'", mapKey.Interface(), s)
}

// UnmarshalWithTypeIDs is similar to json.Unmarshal, but any interface field
// met in a structure is unserialized as a structure containing the type
// identifier and the value. It allows to unmarshal a JSON (serialized
// by MarshalWithTypeIDs) without loosing typing.
//
// This function is the inverse function for MarshalWithTypeIDs.
//
// NOTE! This is not a drop-in replacement for standard json.Unmarshal.
//
//	It has incompatible behavior.
func UnmarshalWithTypeIDs(b []byte, dst any, newByTypeIDer NewByTypeIDer) error {
	// TODO: use encoding/json.Decoder instead of github.com/tidwall/gjson
	return unmarshal(gjson.ParseBytes(b), reflect.ValueOf(dst), newByTypeIDer)
}

func unmarshal(obj gjson.Result, v reflect.Value, newByTypeIDer NewByTypeIDer) error {
	// How the function works:
	//
	// We are interested only about structures (and their fields),
	// everything else supposed to be handled by standard "encoding/json" package.
	// So we use reflection to go through the value and handle values accordingly.
	//
	// If during iteration through structure fields we meet an interface,
	// we use NewByTypeIDer to create a sample, and then standard "json.Unmarshal" to fill it.

	if v.Kind() != reflect.Pointer {
		return fmt.Errorf("expected a pointer destination, but got %T instead", v.Interface())
	}

	if !v.Elem().IsValid() {
		// Some field may contain a typed nil. But we need to fill the value, so
		// creating an empty value.
		v.Set(reflect.New(v.Type().Elem()))
	}

	switch v.Elem().Kind() {
	case reflect.Interface:
		// unwrapping the interface
		return unmarshal(obj, v.Elem(), newByTypeIDer)
	case reflect.Pointer:
		return unmarshal(obj, v.Elem(), newByTypeIDer)
	case reflect.Map:
		v = v.Elem()

		// delete all entries from the current map
		iterator := v.MapRange()
		for iterator.Next() {
			v.SetMapIndex(iterator.Key(), reflect.Value{})
		}

		// parse entries to the map
		var err error
		keyType := v.Type().Key()
		valueType := v.Type().Elem()
		// iterating through all entries of the associative array
		obj.ForEach(func(key, value gjson.Result) bool {
			keyValue := reflect.New(keyType).Elem()
			err = unstringifyMapKey(keyValue, key.Str)
			if err != nil {
				err = fmt.Errorf("unable to unstringify key value '%s': %w", key.Str, err)
				return false
			}

			valueValue := reflect.New(valueType).Elem()
			err = unmarshalTo(valueValue, valueType, value, newByTypeIDer)
			if err != nil {
				err = fmt.Errorf("unable to unmarshal JSON '%s' of entry with key '%s': %w", value, key, err)
				return false
			}

			if v.IsNil() {
				// Got a nil map, initializing:
				v.Set(reflect.MakeMap(v.Type()))
			}
			v.SetMapIndex(keyValue, valueValue)
			return true
		})
		return err
	case reflect.Slice, reflect.Array:
		// conversion for slices and arrays is not supported, yet
		return json.Unmarshal([]byte(obj.Raw), v.Interface())
	case reflect.Struct:
		v = v.Elem()
		t := v.Type()

		// indexMap is a map of JSON field name to structure field index (could be used with Field method in reflection)
		indexMap := map[string]int{}
		for i := 0; i < v.NumField(); i++ {
			fT := t.Field(i)

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

			indexMap[jsonFieldName] = i
		}

		var err error
		// Iterating through fields of the structure provided in the JSON:
		obj.ForEach(func(key, value gjson.Result) bool {
			fieldIndex, ok := indexMap[string(key.Str)]
			if !ok {
				// we have no such field in our struct
				return true
			}

			fT := t.Field(fieldIndex)
			fV := v.Field(fieldIndex)

			if fT.PkgPath != "" {
				// unexported
				return true
			}

			err = unmarshalTo(fV, fT.Type, value, newByTypeIDer)
			if err != nil {
				err = fmt.Errorf("unable to unmarshal JSON '%s' of field '%s': %w", value, key, err)
				return false
			}
			return true
		})
		return err
	}

	// Everything else:
	return json.Unmarshal([]byte(obj.Raw), v.Interface())
}

func unmarshalTo(
	out reflect.Value,
	outType reflect.Type,
	value gjson.Result,
	newByTypeIDer NewByTypeIDer,
) error {
	// By default unmarshaling directly to the field value
	contentOut := out.Addr()

	switch outType.Kind() {
	case reflect.Pointer:
		if value.Type == gjson.Null {
			out.Set(reflect.Zero(outType))
			return nil
		}
	case reflect.Interface:
		// The field is an interface. It is required to generate a value
		// of the type, defined by TypeID and unmarshal the content into it.

		// Checking if it should be the untyped-nil value
		if value.Type == gjson.Null {
			out.Set(reflect.New(outType).Elem())
			return nil
		}

		// Getting the TypeID

		m := value.Map()
		if len(m) != 1 {
			return fmt.Errorf("expected exactly one value, but got %d", len(m))
		}
		var (
			typeID        string
			valueUnparsed gjson.Result
			typedValuePtr any
		)
		// There will be only one value, unpacking it:
		for typeID, valueUnparsed = range m {
		}

		// Generating a value with type corresponding to the TypeID

		typedValuePtr, err := newByTypeIDer.NewByTypeID(TypeID(typeID))
		if err != nil {
			return fmt.Errorf("unable to construct an instance of value for TypeID '%s': %w", typeID, err)
		}

		// Setting to unmarshal the content (JSON) to the generated value

		contentOut = reflect.ValueOf(typedValuePtr)
		value = valueUnparsed
	}

	// unmarshaling the content
	err := unmarshal(value, contentOut, newByTypeIDer)
	if err != nil {
		return fmt.Errorf("unable to unmarshal: %w", err)
	}

	if outType.Kind() == reflect.Interface {
		// Since it was an interface and we generated a dedicated variable to unmarshal to,
		// no we need to set the final value to the structure field.

		// There are few cases possible:
		switch {
		case contentOut.Elem().Type().AssignableTo(outType):
			// This is the main case. Here we just set the resulting
			// value the the field.
			out.Set(contentOut.Elem())
		case contentOut.Type().AssignableTo(outType):
			// Some TypeID handlers may dereference pointers, and
			// because of this we need to get back to references,
			// so we remove "Elem()"
			out.Set(contentOut)
		default:
			return fmt.Errorf("internal error: do not know how to assign %T to %s", contentOut.Elem(), outType)
		}
	}

	return nil
}
