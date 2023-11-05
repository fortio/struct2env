# struct2env
Convert between go structures to environment variable and back (for structured config &lt;-> shell env)

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
TST_FOO="a\nfoo with \" quotes and \\ and '"
TST_BAR="42str"
TST_A_SPECIAL_BLAH="42"
TST_A_BOOL=true
TST_HTTP_SERVER="http://localhost:8080"
TST_INT_POINTER="199"
```

Using
```go
struct2env.ToShellWithPrefix("TST_", struct2env.StructToEnvVars(foo))
```
