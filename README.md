# struct2env
Convert between go structures to environment variables and back (for structured config &lt;-> shell env)

There are many go packages that are doing environment to go struct config (for instance https://github.com/kelseyhightower/envconfig) but I didn't find one doing the inverse and we needed to set a bunch of environment variables for shell and other tools to get some configuration structured as JSON and Go object, so this was born. For symetry the reverse was also added (history of commit on https://github.com/fortio/dflag/pull/50/commits)

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

Turns into
```shell
TST_FOO='a
foo with $, `, " quotes, \ and '\'' quotes'
TST_BAR='42str'
TST_A_SPECIAL_BLAH='42'
TST_A_BOOL=true
TST_HTTP_SERVER='http://localhost:8080'
TST_INT_POINTER='199'
TST_FLOAT_POINTER='3.14'
```

Using
```go
kv, errs := struct2env.StructToEnvVars(foo)
txt := struct2env.ToShellWithPrefix("TST_", kv)
```

Type conversions:

- Most primitive type to their string representation, single quote (') escaped.
- []byte are encoded as base64
- time.Time are formatted as RFC3339
- time.Duration are in (floating point) seconds.
