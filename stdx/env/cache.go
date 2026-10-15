package env

import (
	"reflect"
	"strings"
	"sync"
	"time"
)

type fieldInfo struct {
	key        string
	ignore     bool
	required   bool
	defaultVal string
}

type cache struct {
	mu      sync.RWMutex
	fields  map[reflect.Type][]fieldInfo
	conv    map[reflect.Type]converter
	hasDefs map[reflect.Type]bool
}

var sharedCache = &cache{
	fields:  make(map[reflect.Type][]fieldInfo),
	conv:    make(map[reflect.Type]converter),
	hasDefs: make(map[reflect.Type]bool),
}

type converter func(string) (reflect.Value, error)

func parseFieldInfo(field reflect.StructField) fieldInfo {
	info := fieldInfo{key: field.Name}

	tag, ok := field.Tag.Lookup("env")
	if !ok {
		return info
	}

	parts := strings.Split(tag, ",")
	info.key = parts[0]

	if info.key == "-" {
		info.ignore = true
		return info
	}

	for _, opt := range parts[1:] {
		opt = strings.TrimSpace(opt)
		switch {
		case opt == "required":
			info.required = true
		case strings.HasPrefix(opt, "default="):
			info.defaultVal = opt[len("default="):]
		}
	}

	return info
}

func (c *cache) structFields(t reflect.Type) []fieldInfo {
	c.mu.RLock()
	infos, ok := c.fields[t]
	c.mu.RUnlock()
	if ok {
		return infos
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if infos, ok := c.fields[t]; ok {
		return infos
	}

	infos = make([]fieldInfo, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		infos[i] = parseFieldInfo(t.Field(i))
	}
	c.fields[t] = infos
	return infos
}

func (c *cache) converter(t reflect.Type) converter {
	c.mu.RLock()
	conv := c.conv[t]
	c.mu.RUnlock()
	if conv != nil {
		return conv
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if conv, ok := c.conv[t]; ok {
		return conv
	}

	conv = builtinConverter(t)
	if conv != nil {
		c.conv[t] = conv
	}
	return conv
}

func builtinConverter(t reflect.Type) converter {
	switch t.Kind() {
	case reflect.String:
		return func(v string) (reflect.Value, error) {
			return reflect.ValueOf(v), nil
		}
	case reflect.Bool:
		return func(v string) (reflect.Value, error) {
			switch v {
			case "1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On":
				return reflect.ValueOf(true), nil
			case "0", "false", "FALSE", "False", "no", "NO", "No", "off", "OFF", "Off":
				return reflect.ValueOf(false), nil
			default:
				return reflect.Value{}, parseError(t, v)
			}
		}
	case reflect.Int:
		return intConverter[int](64)
	case reflect.Int8:
		return intConverter[int8](64)
	case reflect.Int16:
		return intConverter[int16](64)
	case reflect.Int32:
		return intConverter[int32](64)
	case reflect.Int64:
		if t == durationType {
			return func(v string) (reflect.Value, error) {
				d, err := time.ParseDuration(v)
				if err != nil {
					return reflect.Value{}, parseError(t, v)
				}
				return reflect.ValueOf(d), nil
			}
		}
		return intConverter[int64](64)
	case reflect.Uint:
		return uintConverter[uint](64)
	case reflect.Uint8:
		return uintConverter[uint8](64)
	case reflect.Uint16:
		return uintConverter[uint16](64)
	case reflect.Uint32:
		return uintConverter[uint32](64)
	case reflect.Uint64:
		return uintConverter[uint64](64)
	case reflect.Float32:
		return floatConverter[float32](32)
	case reflect.Float64:
		return floatConverter[float64](64)
	}
	return nil
}

func parseError(t reflect.Type, v string) error {
	return &strconvErr{typ: t, val: v}
}

type strconvErr struct {
	typ reflect.Type
	val string
}

func (e *strconvErr) Error() string {
	return "cannot parse " + e.val + " as " + e.typ.String()
}

func intConverter[T intVal](bitSize int) converter {
	return func(v string) (reflect.Value, error) {
		n, err := parseInt(v, bitSize)
		if err != nil {
			return reflect.Value{}, parseError(reflect.TypeOf(T(0)), v)
		}
		return reflect.ValueOf(T(n)), nil
	}
}

func uintConverter[T uintVal](bitSize int) converter {
	return func(v string) (reflect.Value, error) {
		n, err := parseUint(v, bitSize)
		if err != nil {
			return reflect.Value{}, parseError(reflect.TypeOf(T(0)), v)
		}
		return reflect.ValueOf(T(n)), nil
	}
}

func floatConverter[T floatVal](bitSize int) converter {
	return func(v string) (reflect.Value, error) {
		n, err := parseFloat(v, bitSize)
		if err != nil {
			return reflect.Value{}, parseError(reflect.TypeOf(T(0)), v)
		}
		return reflect.ValueOf(T(n)), nil
	}
}

type intVal interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type uintVal interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

type floatVal interface {
	~float32 | ~float64
}

var durationType = reflect.TypeOf(time.Duration(0))

func isBuiltinStruct(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t == durationType
}

func (c *cache) hasDefaults(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	if isBuiltinStruct(t) {
		return false
	}

	c.mu.RLock()
	result, ok := c.hasDefs[t]
	c.mu.RUnlock()
	if ok {
		return result
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if result, ok := c.hasDefs[t]; ok {
		return result
	}

	result = c.scanDefaults(t)
	c.hasDefs[t] = result
	return result
}

func (c *cache) scanDefaults(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("env")
		if tag != "" && tag != "-" {
			parts := strings.Split(tag, ",")
			for _, part := range parts[1:] {
				if strings.HasPrefix(strings.TrimSpace(part), "default=") {
					return true
				}
			}
		}

		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && !isBuiltinStruct(ft) &&
			!reflect.PtrTo(ft).Implements(textUnmarshalerType) &&
			!reflect.PtrTo(ft).Implements(binaryUnmarshalerType) {
			if c.scanDefaults(ft) {
				return true
			}
		}
	}
	return false
}
