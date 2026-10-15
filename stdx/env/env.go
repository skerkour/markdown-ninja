package env

import (
	"encoding/base64"
	"fmt"
	"os"
	"reflect"
	"strings"
)

func Unmarshal(env map[string]string, dst any) error {
	return UnmarshalWithPrefix("", env, dst)
}

func UnmarshalWithPrefix(prefix string, env map[string]string, dst any) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("env: destination must be a pointer to a struct")
	}
	v = v.Elem()
	return decode(v, prefix, env)
}

// Load reads all environment variables from the process into a map.
func Load() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		k, v, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[k] = v
	}
	return env
}

func decode(v reflect.Value, prefix string, env map[string]string) error {
	t := v.Type()
	infos := sharedCache.structFields(t)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		if !fieldVal.CanSet() {
			continue
		}

		info := infos[i]
		if info.ignore {
			continue
		}

		key := info.key
		if key == "" {
			key = field.Name
		}
		key = prefix + key

		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if ft.Kind() == reflect.Struct && !isBuiltinStruct(field.Type) {
			if field.Type.Kind() == reflect.Ptr {
				if fieldVal.IsNil() {
					if !hasMatchingPrefix(env, key+"_") && !sharedCache.hasDefaults(field.Type) {
						continue
					}
					fieldVal.Set(reflect.New(ft))
				}
				fieldVal = fieldVal.Elem()
			}
			if !reflect.PtrTo(ft).Implements(textUnmarshalerType) &&
				!reflect.PtrTo(ft).Implements(binaryUnmarshalerType) {
				if err := decode(fieldVal, key+"_", env); err != nil {
					return err
				}
				continue
			}
		}

		val, ok := env[key]
		if !ok && info.defaultVal != "" {
			val = info.defaultVal
			ok = true
		}
		if !ok && info.required {
			return fmt.Errorf("env: required key %q not set", key)
		}
		if !ok {
			continue
		}

		if err := setField(fieldVal, val); err != nil {
			return fmt.Errorf("env: %s: %w", key, err)
		}
	}
	return nil
}

func setField(fieldVal reflect.Value, val string) error {
	if fieldVal.Kind() == reflect.Ptr {
		if fieldVal.IsNil() {
			fieldVal.Set(reflect.New(fieldVal.Type().Elem()))
		}
		fieldVal = fieldVal.Elem()
	}

	if isTextUnmarshaler(fieldVal) {
		return fieldVal.Addr().Interface().(interface{ UnmarshalText([]byte) error }).UnmarshalText([]byte(val))
	}

	if isBinaryUnmarshaler(fieldVal) {
		return fieldVal.Addr().Interface().(interface{ UnmarshalBinary([]byte) error }).UnmarshalBinary([]byte(val))
	}

	// support for []byte
	if fieldVal.Kind() == reflect.Slice && fieldVal.Type().Elem().Kind() == reflect.Uint8 {
		b, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			return fmt.Errorf("cannot decode base64 %q: %w", val, err)
		}
		fieldVal.Set(reflect.ValueOf(b))
		return nil
	}

	if fieldVal.Kind() == reflect.Slice {
		return setSlice(fieldVal, val)
	}

	conv := sharedCache.converter(fieldVal.Type())
	if conv != nil {
		converted, err := conv(val)
		if err != nil {
			return fmt.Errorf("cannot convert %q to %s: %w", val, fieldVal.Type(), err)
		}
		if converted.Type() != fieldVal.Type() {
			converted = converted.Convert(fieldVal.Type())
		}
		fieldVal.Set(converted)
		return nil
	}

	return fmt.Errorf("env: unsupported type %s", fieldVal.Type())
}

func isTextUnmarshaler(v reflect.Value) bool {
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return reflect.PtrTo(t).Implements(textUnmarshalerType)
}

var (
	textUnmarshalerType   = reflect.TypeOf((*interface{ UnmarshalText([]byte) error })(nil)).Elem()
	binaryUnmarshalerType = reflect.TypeOf((*interface{ UnmarshalBinary([]byte) error })(nil)).Elem()
)

func isBinaryUnmarshaler(v reflect.Value) bool {
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return reflect.PtrTo(t).Implements(binaryUnmarshalerType)
}

func hasMatchingPrefix(env map[string]string, prefix string) bool {
	for key := range env {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
