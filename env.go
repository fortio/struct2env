// Package env provides conversion from structure to and from environment variables.
//
// It supports converting struct fields to environment variables using field tags,
// handling different data types, and transforming strings between different case
// conventions, which is useful for generating or parsing environment variables,
// JSON tags, or command line flags.
//
// The package also defines several case conversion functions that aid in manipulating
// strings to fit conventional casing for various programming and configuration contexts.
// Additionally, it provides functions to serialize structs into slices of key-value pairs
// where the keys are derived from struct field names transformed to upper snake case by default,
// or specified explicitly via struct field tags.
//
// It also includes functionality to deserialize environment variables back into
// struct fields, handling pointers and nested structs appropriately, as well as providing
// shell-compatible output for environment variable definitions.
//
// The package leverages reflection to dynamically handle arbitrary struct types,
// and has 0 dependencies.
package struct2env

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// Split strings into words, using CamelCase/camelCase/CAMELCase rules.
func SplitByCase(input string) []string {
	if input == "" {
		return nil
	}
	var words []string
	var buffer strings.Builder
	runes := []rune(input)

	for i := 0; i < len(runes); i++ {
		first := (i == 0)
		last := (i == len(runes)-1)
		if !first && unicode.IsUpper(runes[i]) {
			if !last && unicode.IsLower(runes[i+1]) || unicode.IsLower(runes[i-1]) {
				words = append(words, buffer.String())
				buffer.Reset()
			}
		}
		buffer.WriteRune(runes[i])
	}
	words = append(words, buffer.String())
	return words
}

// CamelCaseToUpperSnakeCase converts a string from camelCase or CamelCase
// to UPPER_SNAKE_CASE. Handles cases like HTTPServer -> HTTP_SERVER and
// httpServer -> HTTP_SERVER. Good for environment variables.
func CamelCaseToUpperSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	words := SplitByCase(s)
	// ToUpper + Join by _
	return strings.ToUpper(strings.Join(words, "_"))
}

// CamelCaseToLowerSnakeCase converts a string from camelCase or CamelCase
// to lowe_snake_case. Handles cases like HTTPServer -> http_server.
// Good for JSON tags for instance.
func CamelCaseToLowerSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	words := SplitByCase(s)
	// ToLower + Join by _
	return strings.ToLower(strings.Join(words, "_"))
}

// CamelCaseToLowerKebabCase converts a string from camelCase or CamelCase
// to lower-kebab-case. Handles cases like HTTPServer -> http-server.
// Good for command line flags for instance.
func CamelCaseToLowerKebabCase(s string) string {
	if s == "" {
		return ""
	}
	words := SplitByCase(s)
	// ToLower and join by -
	return strings.ToLower(strings.Join(words, "-"))
}

type KeyValue struct {
	Key   string
	Value string // Already quoted/escaped.
}

func (kv KeyValue) String() string {
	return fmt.Sprintf("%s=%s", kv.Key, kv.Value)
}

func ToShell(kvl []KeyValue) string {
	return ToShellWithPrefix("", kvl)
}

// This convert the key value pairs to bourne shell syntax (vs newer bash export FOO=bar).
func ToShellWithPrefix(prefix string, kvl []KeyValue) string {
	var sb strings.Builder
	keys := make([]string, 0, len(kvl))
	for _, kv := range kvl {
		sb.WriteString(prefix)
		sb.WriteString(kv.String())
		sb.WriteRune('\n')
		keys = append(keys, prefix+kv.Key)
	}
	sb.WriteString("export ")
	sb.WriteString(strings.Join(keys, " "))
	sb.WriteRune('\n')
	return sb.String()
}

func SerializeValue(value interface{}) string {
	switch v := value.(type) {
	case bool:
		res := "false"
		if v {
			res = "true"
		}
		return res
	case string:
		return strconv.Quote(v)
	default:
		return strconv.Quote(fmt.Sprint(value))
	}
}

// StructToEnvVars converts a struct to a map of environment variables.
// The struct can have a `env` tag on each field.
// The tag should be in the format `env:"ENV_VAR_NAME"`.
// The tag can also be `env:"-"` to exclude the field from the map.
// If the field is exportable and the tag is missing we'll use the field name
// converted to UPPER_SNAKE_CASE (using CamelCaseToUpperSnakeCase()) as the
// environment variable name.
func StructToEnvVars(s interface{}) ([]KeyValue, []error) {
	var allErrors []error
	var allKeyValVals []KeyValue
	return structToEnvVars(allKeyValVals, allErrors, "", s)
}

// Appends additional results and errors to incoming envVars and allErrors and return them (for recursion).
func structToEnvVars(envVars []KeyValue, allErrors []error, prefix string, s interface{}) ([]KeyValue, []error) {
	v := reflect.ValueOf(s)
	// if we're passed a pointer to a struct instead of the struct, let that work too
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		err := fmt.Errorf("unexpected kind %v, expected a struct", v.Kind())
		allErrors = append(allErrors, err)
		return envVars, allErrors
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)
		tag := fieldType.Tag.Get("env")
		if tag == "-" {
			continue
		}
		if fieldType.Anonymous {
			// Recurse
			envVars, allErrors = structToEnvVars(envVars, allErrors, "", v.Field(i).Interface())
			continue
		}
		if tag == "" {
			tag = CamelCaseToUpperSnakeCase(fieldType.Name)
		}
		fieldValue := v.Field(i)
		stringValue := ""
		switch fieldValue.Kind() { //nolint: exhaustive // we have default: for the other cases
		case reflect.Ptr:
			if !fieldValue.IsNil() {
				fieldValue = fieldValue.Elem()
				stringValue = SerializeValue(fieldValue.Interface())
			}
		case reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
			// log.LogVf("Skipping field %s of type %v, not supported", fieldType.Name, fieldType.Type)
			continue
		case reflect.Struct:
			// Recurse with prefix
			envVars, allErrors = structToEnvVars(envVars, allErrors, tag+"_", fieldValue.Interface())
			continue
		default:
			value := fieldValue.Interface()
			stringValue = SerializeValue(value)
		}
		envVars = append(envVars, KeyValue{Key: prefix + tag, Value: stringValue})
	}
	return envVars, allErrors
}

func setPointer(fieldValue reflect.Value) reflect.Value {
	// Ensure we have a pointer to work with, allocate if nil.
	if fieldValue.IsNil() {
		fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
	}
	// Get the element the pointer is pointing to.
	return fieldValue.Elem()
}

func checkEnv(envName, fieldName string, fieldValue reflect.Value) (*string, error) {
	val, found := os.LookupEnv(envName)
	if !found {
		// log.LogVf("%q not set for %s", envName, fieldName)
		return nil, nil //nolint:nilnil
	}
	// log.Infof("Found %s=%q to set %s", envName, val, fieldName)
	if !fieldValue.CanSet() {
		err := fmt.Errorf("can't set %s (found %s=%q)", fieldName, envName, val)
		return nil, err
	}
	return &val, nil
}

func SetFromEnv(prefix string, s interface{}) []error {
	return setFromEnv(nil, prefix, s)
}

func setFromEnv(allErrors []error, prefix string, s interface{}) []error {
	// TODO: this is quite similar in structure to structToEnvVars() - can it be refactored with
	// passing setter vs getter function and share the same iteration (yet a little bit of copy is the go way too)
	v := reflect.ValueOf(s)
	// if we're passed a pointer to a struct instead of the struct, let that work too
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		err := fmt.Errorf("unexpected kind %v, expected a struct", v.Kind())
		allErrors = append(allErrors, err)
		return allErrors
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldType := t.Field(i)
		tag := fieldType.Tag.Get("env")
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = CamelCaseToUpperSnakeCase(fieldType.Name)
		}
		envName := prefix + tag
		fieldValue := v.Field(i)

		kind := fieldValue.Kind()

		if kind == reflect.Struct {
			// Recurse with prefix
			if fieldValue.CanAddr() { // Check if we can get the address
				SetFromEnv(envName+"_", fieldValue.Addr().Interface())
			} else {
				err := fmt.Errorf("cannot take the address of %s to recurse", fieldType.Name)
				allErrors = append(allErrors, err)
			}
			continue
		}
		val, err := checkEnv(envName, fieldType.Name, fieldValue)
		if err != nil {
			allErrors = append(allErrors, err)
			continue
		}
		if val == nil {
			continue
		}
		envVal := *val

		// Handle pointer fields separately
		if kind == reflect.Ptr {
			kind = fieldValue.Type().Elem().Kind()
			fieldValue = setPointer(fieldValue)
		}
		switch kind { //nolint: exhaustive // we have default: for the other cases
		case reflect.String:
			fieldValue.SetString(envVal)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			var ev int64
			ev, err = strconv.ParseInt(envVal, 10, fieldValue.Type().Bits())
			if err == nil {
				fieldValue.SetInt(ev)
			}
		case reflect.Float32, reflect.Float64:
			var ev float64
			ev, err = strconv.ParseFloat(envVal, fieldValue.Type().Bits())
			if err == nil {
				fieldValue.SetFloat(ev)
			}
		case reflect.Bool:
			var ev bool
			ev, err = strconv.ParseBool(envVal)
			if err == nil {
				fieldValue.SetBool(ev)
			}
		default:
			err = fmt.Errorf("unsupported type %v to set from %s=%q", kind, envName, envVal)
		}
		if err != nil {
			allErrors = append(allErrors, err)
		}
	}
	return allErrors
}
