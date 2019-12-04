# License

Copyright 2019 Tad Lebeck

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

# testutils

Test utilities

- [Clone](#clone) Deep copy support
- [Introspect](#introspect) Examine source symbols
- [Mock matchers](#mock-matchers) Support for gomock
- [Test Logger](#test-logger) A logger to use in unit tests

## Clone
This provides support to "deep-copy" an object. (There is no native support in Go).

```
  var src SomeType{} // initialized
  var dst Type
  testutils.Clone(src, &dst)
```
Note that the *dst* variable must be passed as a pointer.
There are some cases that seem to fail, mostly involving copying objects via their abstract interfaces; the underlying [Gob encoder](https://golang.org/pkg/encoding/gob/) provides some sort of registry mechanism but we avoided the issue by careful construction of test cases.

## Introspect
This provides support to examine source symbols from a Go source file.

Basic (numeric, string) constant symbols can be examined as follows:
```
  bc, err := testutils.FetchBasicConstantsFromGoFile(fileName)
  sMap := bc.GetStringConstants(nil)
  iMap := bc.GetIntConstants(regexp.MustCompile("^Default[A-Z]"))
```

## Mock matchers

This provides a large set of gomock "Matchers" that can be used against mocked calls.

## Test Logger
A [go-logging](https://github.com/op/go-logging) based logger to use in unit tests.
Usage:
```
func TestCode(t *testing.T) {
    // get a new test logger
    tl := testutils.NewTestLogger(t)

    // Flush writes accumulated log records to t.Log before returning
    // They are visible only if tests fail or verbose is enabled
    defer tl.Flush()

    // Logger returns a go-logging in-memory test logger
    tl.Logger().Info("Log message")

    // tests that use the logger ...

    // CountPattern counts the occurrence of a pattern in records since the last Flush()
    assert.Equal(1, tl.CountPattern("regular expression"))

    // Flush can be called multiple times
    // It only displays messages since the last call or from beginning if not called previously
    tl.Flush()

    // more tests ...

    // Iterate walks through all the records since the beginning.
    tl.Iterate(func(num uint64, msg string) { fmt.Printf("%d %s\n", num, msg) })
}
```
If your unit test hangs and *go test* times out you will not get any output.
In this case set the **tl.LogToConsole** variable to true.
You could also try testing with *go test -v*.
