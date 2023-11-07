package struct2env

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSplitByCase(t *testing.T) {
	tests := []struct {
		in  string
		out []string
	}{
		{"", nil},
		{"http2Server", []string{"http2", "Server"}},
		{"HTTPSServer42", []string{"HTTPS", "Server42"}},
		{"1", []string{"1"}},
		{"1a", []string{"1a"}},
		{"1a2Bb", []string{"1a2", "Bb"}}, // note 1a2B doesn't split
		{"a", []string{"a"}},
		{"A", []string{"A"}},
		{"Ab", []string{"Ab"}},
		{"AB", []string{"AB"}},
		{"AB", []string{"AB"}},
		{"ABC", []string{"ABC"}},
		{"ABCd", []string{"AB", "Cd"}},
		{"aa", []string{"aa"}},
		{"aaA", []string{"aa", "A"}},
		{"AAb", []string{"A", "Ab"}},
		{"aaBbbCcc", []string{"aa", "Bbb", "Ccc"}},
		{"AaBbbCcc", []string{"Aa", "Bbb", "Ccc"}},
		{"AABbbCcc", []string{"AA", "Bbb", "Ccc"}},
	}
	for _, test := range tests {
		got := SplitByCase(test.in)
		if !reflect.DeepEqual(got, test.out) {
			t.Errorf("mismatch for %q: got %v expected %v", test.in, got, test.out)
		}
	}
}

// TestCamelCaseToSnakeCase tests the CamelCaseToUpperSnakeCase and CamelCaseToLowerSnakeCase functions.
func TestCamelCaseToSnakeCase(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"", ""},
		{"a", "A"},
		{"A", "A"},
		{"Ab", "AB"},
		{"AB", "AB"},
		{"ABCd", "AB_CD"},
		{"aa", "AA"},
		{"aaA", "AA_A"},
		{"AAb", "A_AB"},
		{"aaBbbCcc", "AA_BBB_CCC"},
		{"http2Server", "HTTP2_SERVER"},
		{"HTTPSServer42", "HTTPS_SERVER42"},
	}
	for _, test := range tests {
		if got := CamelCaseToUpperSnakeCase(test.in); got != test.out {
			t.Errorf("for %q expected upper %q and got %q", test.in, test.out, got)
		}
		lower := strings.ToLower(test.out)
		if got := CamelCaseToLowerSnakeCase(test.in); got != lower {
			t.Errorf("for %q expected lower %q and got %q", test.in, lower, got)
		}
	}
}

func TestCamelCaseToLowerKebabCase(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"", ""},
		{"a", "a"},
		{"A", "a"},
		{"Ab", "ab"},
		{"AB", "ab"},
		{"ABCd", "ab-cd"},
		{"aa", "aa"},
		{"aaA", "aa-a"},
		{"AAb", "a-ab"},
		{"aaBbbCcc", "aa-bbb-ccc"},
		{"http2Server", "http2-server"},
		{"HTTPSServer42", "https-server42"},
	}
	for _, test := range tests {
		if got := CamelCaseToLowerKebabCase(test.in); got != test.out {
			t.Errorf("for %q expected %q and got %q", test.in, test.out, got)
		}
	}
}

type Embedded struct {
	InnerA string
	InnerB string
}

type HiddenEmbedded struct {
	HA string
	HB string
}

type FooConfig struct {
	Foo          string
	Bar          string
	Blah         int `env:"A_SPECIAL_BLAH"`
	ABool        bool
	NotThere     int `env:"-"`
	HTTPServer   string
	IntPointer   *int
	FloatPointer *float64
	WontShowYet  map[string]string
	Embedded
	HiddenEmbedded `env:"-"`
	RecurseHere    Embedded
	SomeBinary     []byte
	Dur            time.Duration
	TS             time.Time
}

func TestStructToEnvVars(t *testing.T) {
	intV := 199
	foo := FooConfig{
		Foo:          "a newline:\nfoo with $X, `backticks`, \" quotes and \\ and ' in middle and end '",
		Bar:          "42str",
		Blah:         42,
		ABool:        true,
		NotThere:     13,
		HTTPServer:   "http://localhost:8080",
		IntPointer:   &intV,
		FloatPointer: nil,
		RecurseHere: Embedded{
			InnerA: "rec a",
			InnerB: "rec b",
		},
		SomeBinary: []byte{0, 1, 2},
		Dur:        1*time.Hour + 100*time.Millisecond,
		TS:         time.Date(1998, time.November, 5, 14, 30, 0, 0, time.UTC),
	}
	foo.InnerA = "inner a"
	foo.InnerB = "inner b"
	empty, errors := StructToEnvVars(42) // error/empty
	if len(empty) != 0 {
		t.Errorf("expected empty, got %v", empty)
	}
	if len(errors) != 1 {
		t.Errorf("expected errors, got %v", errors)
	}
	envVars, errors := StructToEnvVars(&foo)
	if len(errors) != 0 {
		t.Errorf("expected no error, got %v", errors)
	}
	if len(envVars) != 14 {
		t.Errorf("expected 14 env vars, got %d: %+v", len(envVars), envVars)
	}
	str := ToShellWithPrefix("TST_", envVars)
	//nolint:lll
	expected := `TST_FOO='a newline:
foo with $X, ` + "`backticks`" + `, " quotes and \ and '\'' in middle and end '\'''
TST_BAR='42str'
TST_A_SPECIAL_BLAH='42'
TST_A_BOOL=true
TST_HTTP_SERVER='http://localhost:8080'
TST_INT_POINTER='199'
TST_FLOAT_POINTER=
TST_INNER_A='inner a'
TST_INNER_B='inner b'
TST_RECURSE_HERE_INNER_A='rec a'
TST_RECURSE_HERE_INNER_B='rec b'
TST_SOME_BINARY='AAEC'
TST_DUR=3600.1
TST_TS='1998-11-05T14:30:00Z'
export TST_FOO TST_BAR TST_A_SPECIAL_BLAH TST_A_BOOL TST_HTTP_SERVER TST_INT_POINTER TST_FLOAT_POINTER TST_INNER_A TST_INNER_B TST_RECURSE_HERE_INNER_A TST_RECURSE_HERE_INNER_B TST_SOME_BINARY TST_DUR TST_TS
`
	if str != expected {
		t.Errorf("\n---expected:---\n%s\n---got:---\n%s", expected, str)
	}
	// NUL in string
	type Cfg struct {
		Foo string
	}
	cfg := Cfg{Foo: "ABC\x00DEF"}
	envVars, errors = StructToEnvVars(&cfg)
	if len(errors) != 1 {
		t.Errorf("Should have had error with embedded NUL")
	}
	if envVars[0].Key != "FOO" {
		t.Errorf("Expecting key to be present %v", envVars)
	}
	if envVars[0].QuotedValue != "" {
		t.Errorf("Expecting value to be empty %v", envVars)
	}
}

func TestSetFromEnv(t *testing.T) {
	foo := FooConfig{}
	envs := map[string]string{
		"TST2_FOO":                  "another\nfoo",
		"TST2_BAR":                  "bar",
		"TST2_RECURSE_HERE_INNER_B": "in1",
		"TST2_A_SPECIAL_BLAH":       "31",
		"TST2_A_BOOL":               "1",
		"TST2_FLOAT_POINTER":        "5.75",
		"TST2_INT_POINTER":          "73",
		"TST2_SOME_BINARY":          "QUJDAERFRg==",
	}
	lookup := func(key string) (string, bool) {
		value, found := envs[key]
		return value, found
	}
	errors := SetFrom(lookup, "TST2_", &foo)
	if len(errors) != 0 {
		t.Errorf("Unexpectedly got errors :%v", errors)
	}
	if foo.Foo != "another\nfoo" || foo.Bar != "bar" || foo.RecurseHere.InnerB != "in1" || foo.Blah != 31 || foo.ABool != true {
		t.Errorf("Mismatch in object values, got: %+v", foo)
	}
	if foo.IntPointer == nil || *foo.IntPointer != 73 {
		t.Errorf("IntPointer not set correctly: %v %v", foo.IntPointer, *foo.IntPointer)
	}
	if foo.FloatPointer == nil || *foo.FloatPointer != 5.75 {
		t.Errorf("FloatPointer not set correctly: %v %v", foo.FloatPointer, *foo.FloatPointer)
	}
	if string(foo.SomeBinary) != "ABC\x00DEF" {
		t.Errorf("Base64 decoding not working for []byte field: %q", string(foo.SomeBinary))
	}
}
