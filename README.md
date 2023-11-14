# struct2env

Convert between go structures to environment variables and back (for structured config &lt;-> shell env and to kubernetes YAML env pod spec)

There are many go packages that are doing environment to go struct config (for instance https://github.com/kelseyhightower/envconfig) but I didn't find one doing the inverse and we needed to set a bunch of environment variables for shell and other tools to get some configuration structured as JSON and Go object, so this was born. For symmetry the reverse was also added.

A bit later the `ToYamlWithPrefix()` was also added as alternative serialization to insert in kubernetes deployment CI templates a common cluster configuration for instance.

Standalone package with 0 dependencies outside of the go standard library. Developed with go 1.20 but tested with go as old as 1.17
but should works with pretty much any go version, as it only depends on reflection and strconv.


The unit test has a fairly extensive example on how:
```go
type FooConfig struct {
	Foo          string
	Bar          string
	Blah         int `env:"A_SPECIAL_BLAH"`
	ABool        bool
	NotThere     int `env:"-"`
	HTTPServer   string
	IntPointer   *int
	FloatPointer *float64
    // ...
}
```

Turns into (from the unit tests)
```shell
TST_FOO='a newline:
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
```

Using
```go
kv, errs := struct2env.StructToEnvVars(foo)
txt := struct2env.ToShellWithPrefix("TST_", kv)
```

Or

```yaml
  - name: Y_FOO
    value: "a newline:\nfoo with $X, `backticks`, \" quotes and \\ and ' in middle and end '"
  - name: Y_BAR
    value: "42str"
  - name: Y_A_SPECIAL_BLAH
    value: "42"
  - name: Y_A_BOOL
    value: true
  - name: Y_HTTP_SERVER
    value: "http://localhost:8080"
  - name: Y_INT_POINTER
    value: "199"
  - name: Y_FLOAT_POINTER
    value: null
  - name: Y_INNER_A
    value: "inner a"
  - name: Y_INNER_B
    value: "inner b"
  - name: Y_RECURSE_HERE_INNER_A
    value: "rec a"
  - name: Y_RECURSE_HERE_INNER_B
    value: "rec b"
  - name: Y_SOME_BINARY
    value: 'AAEC'
  - name: Y_DUR
    value: 3600.1
  - name: Y_TS
    value: "1998-11-05T14:30:00Z"
```

using
```go
kv, errs := struct2env.StructToEnvVars(foo)
txt := struct2env.ToYamlWithPrefix("Y_", kv)
```

Type conversions:

- Most primitive type to their string representation, single quote (') escaped for shell and double quote (") for YAML.
- []byte are encoded as base64
- time.Time are formatted as RFC3339
- time.Duration are in (floating point) seconds.
