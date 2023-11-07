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
	"encoding/base64"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
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

// Intermediate result list from StructToEnvVars(), both the Key and QuotedValue
// must be shell safe/non adversarial as they are emitted as is by String() with = in between.
// Using StructToEnvVars produces safe values even with adversarial input (length notwithstanding).
type KeyValue struct {
	Key         string // Must be safe (is when coming from Go struct names but could be bad with env:).
	QuotedValue string // (Must be) Already quoted/escaped.
}

// Escape characters such as the result string can be embedded as a single argument in a shell fragment
// e.g for ENV_VAR=<value> such as <value> is safe (no $(cmd...) no ` etc`). Will error out if NUL is found
// in the input (use []byte for that and it'll get base64 encoded/decoded).
func ShellQuote(input string) (string, error) {
	if strings.ContainsRune(input, 0) {
		return "", fmt.Errorf("String value %q should not contain NUL", input)
	}
	// To emit a single quote in a single quote enclosed string you have to close the current ' then emit a quote (\'),
	// then reopen the single quote sequence to finish. Note that when the string ends with a quote there is an unnecessary
	// trailing ''.
	return "'" + strings.ReplaceAll(input, "'", `'\''`) + "'", nil
}

func (kv KeyValue) String() string {
	return fmt.Sprintf("%s=%s", kv.Key, kv.QuotedValue)
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

func SerializeValue(value interface{}) (string, error) {
	switch v := value.(type) {
	case bool:
		res := "false"
		if v {
			res = "true"
		}
		return res, nil
	case []byte:
		return ShellQuote(base64.StdEncoding.EncodeToString(v))
	case string:
		return ShellQuote(v)
	case time.Duration:
		return fmt.Sprintf("%g", v.Seconds()), nil
	default:
		return ShellQuote(fmt.Sprint(value))
	}
}

// StructToEnvVars converts a struct to a map of environment variables.
// The struct can have a `env` tag on each field.
// The tag should be in the format `env:"ENV_VAR_NAME"`.
// The tag can also be `env:"-"` to exclude the field from the map.
// If the field is exportable and the tag is missing we'll use the field name
// converted to UPPER_SNAKE_CASE (using CamelCaseToUpperSnakeCase()) as the
// environment variable name.
// []byte are encoded as base64, time.Time are formatted as RFC3339, time.Duration are in (floating point) seconds.
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
		var err error

		if fieldValue.Type() == reflect.TypeOf(time.Time{}) { // other wise we hit the "struct" case below
			timeField := fieldValue.Interface().(time.Time)
			stringValue, err = SerializeValue(timeField.Format(time.RFC3339))
			if err != nil {
				allErrors = append(allErrors, err)
			} else {
				envVars = append(envVars, KeyValue{Key: prefix + tag, QuotedValue: stringValue})
			}
			continue // Continue to the next field
		}

		switch fieldValue.Kind() { //nolint: exhaustive // we have default: for the other cases
		case reflect.Ptr:
			if !fieldValue.IsNil() {
				fieldValue = fieldValue.Elem()
				stringValue, err = SerializeValue(fieldValue.Interface())
			}
		case reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
			// From that list of other types, only support []byte
			if fieldValue.Type().Elem().Kind() == reflect.Uint8 {
				stringValue, err = SerializeValue(fieldValue.Interface())
			} else {
				// log.LogVf("Skipping field %s of type %v, not supported", fieldType.Name, fieldType.Type)
				continue
			}
		case reflect.Struct:
			// Recurse with prefix
			envVars, allErrors = structToEnvVars(envVars, allErrors, tag+"_", fieldValue.Interface())
			continue
		default:
			if !fieldValue.CanInterface() {
				err = fmt.Errorf("can't interface %s", fieldType.Name)
			} else {
				value := fieldValue.Interface()
				stringValue, err = SerializeValue(value)
			}
		}
		envVars = append(envVars, KeyValue{Key: prefix + tag, QuotedValue: stringValue})
		if err != nil {
			allErrors = append(allErrors, err)
		}
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

func checkEnv(envLookup EnvLookup, envName, fieldName string, fieldValue reflect.Value) (*string, error) {
	val, found := envLookup(envName)
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

type EnvLookup func(key string) (string, bool)

// Reverse of StructToEnvVars, assumes the same encoding. Using the current os environment variables as source.
func SetFromEnv(prefix string, s interface{}) []error {
	return SetFrom(os.LookupEnv, prefix, s)
}

// Reverse of StructToEnvVars, assumes the same encoding. Using passed it lookup object that can lookup values by keys.
func SetFrom(envLookup EnvLookup, prefix string, s interface{}) []error {
	return setFromEnv(nil, envLookup, prefix, s)
}

func setFromEnv(allErrors []error, envLookup EnvLookup, prefix string, s interface{}) []error {
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

		// Handle time.Time separately a bit below after we get the value
		if kind == reflect.Struct && fieldType.Type != reflect.TypeOf(time.Time{}) {
			// Recurse with prefix
			if fieldValue.CanAddr() { // Check if we can get the address
				allErrors = setFromEnv(allErrors, envLookup, envName+"_", fieldValue.Addr().Interface())
			} else {
				err := fmt.Errorf("cannot take the address of %s to recurse", fieldType.Name)
				allErrors = append(allErrors, err)
			}
			continue
		}
		val, err := checkEnv(envLookup, envName, fieldType.Name, fieldValue)
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
		if fieldType.Type == reflect.TypeOf(time.Time{}) {
			var timeField time.Time
			timeField, err = time.Parse(time.RFC3339, envVal)
			if err == nil {
				fieldValue.Set(reflect.ValueOf(timeField))
			} else {
				allErrors = append(allErrors, err)
			}
			continue
		}
		allErrors = setValue(allErrors, fieldType, fieldValue, kind, envName, envVal)
	}
	return allErrors
}

func setValue(
	allErrors []error,
	fieldType reflect.StructField,
	fieldValue reflect.Value,
	kind reflect.Kind,
	envName, envVal string,
) []error {
	var err error
	switch kind { //nolint: exhaustive // we have default: for the other cases
	case reflect.String:
		fieldValue.SetString(envVal)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// if it's a duration, parse it as a float seconds
		if fieldType.Type == reflect.TypeOf(time.Duration(0)) {
			var ev float64
			ev, err = strconv.ParseFloat(envVal, 64)
			if err == nil {
				fieldValue.SetInt(int64(ev * float64(1*time.Second)))
			}
		} else {
			var ev int64
			ev, err = strconv.ParseInt(envVal, 10, fieldValue.Type().Bits())
			if err == nil {
				fieldValue.SetInt(ev)
			}
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
	case reflect.Slice:
		if fieldValue.Type().Elem().Kind() != reflect.Uint8 {
			err = fmt.Errorf("unsupported slice of %v to set from %s=%q", fieldValue.Type().Elem().Kind(), envName, envVal)
		} else {
			var data []byte
			data, err = base64.StdEncoding.DecodeString(envVal)
			fieldValue.SetBytes(data)
		}
	default:
		err = fmt.Errorf("unsupported type %v to set from %s=%q", kind, envName, envVal)
	}
	if err != nil {
		allErrors = append(allErrors, err)
	}
	return allErrors
}
