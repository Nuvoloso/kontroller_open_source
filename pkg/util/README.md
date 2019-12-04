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

# Utilities
- [Contains](#contains) Check for an element in a slice
- [Controller](#controller) Support for models.NuvoService
- [Critical section guard](#critical-section-guard) Support for fair access to a critical section
- [Exec](#exec) Support to launch commands via an interface
- [K8sSizeBytes](#k8ssizebytes) Support for printing Kubernetes byte units
- [Map key support](#map-key-support) Support to extract map keys
- [MsgList](#msglist) Support for models.TimestampedString slices
- [PanicLogger](#paniclogger) Support to log panics in goroutines
- [Queue](#queue) Support for queues of arbitrary element types
- [Rounding Ticker](#rounding-ticker) An self-adjusting periodic timer
- [Request Middleware Counter](#request-middleware-counter) Middleware for the request/response counter
- [SizeBytes](#sizebytes) Support for printing byte units
- [Snapshot List](#snapshot-list) Support to convert a map of snapshots to an ordered list
- [Status Extractor ResponseWriter](#status-extractor-responsewriter) An HTTP ResponseWriter that provides the status code
- [Tag List](#tag-list) An order preserving tag list manipulator
- [Time related utilities](#time-related-utilities)
- [ValidateIPAddress](#validateipaddress) Support validate an IP address string
- [ValueType Parser](#valuetype-parser) Support to parse a model ValueType data structure
- [Worker](#worker) Support for a background task that can be awoken from sleep

## Contains

Tests if a slice contains the specified element.

```
if util.Contains(slice, elem) {
    fmt.Println("The slice contains the specified element")
}
```
## Controller
The *Controller* interface and the associated *Service* data type provide support for
the *models.NuvoService* data structure.  The following methods are available:

- SetState() sets the service state.
- GetState() returns the service state.
- StateString() returns the string representation of the service state.
- Message() adds a new message to the data structure, potentially purging old messages.
- ReplaceMessage() removes messages matching a pattern and then adds a new message, potentially purging old messages.
- ModelObj() returns a pointer to a *models.NuvoService* object which contains the current state.
- Import() initializes the *Service* object from a *models.NuvoService* object.

## Critical section guard
The *CriticalSectionGuard* data type provides fair access to a critical section
of code by maintaining a queue of waiters for the critical section.

To create:
```
  csg := util.NewCriticalSectionGuard()
```
To tear down the guard and flush all pending waiters do the following:
```
  csg.Drain()
```

The following blocks the invoker until it can enter the critical section:
```
   cst, err := csg.Enter("invokerId")
```
The call returns an error if the critical section is closed. Otherwise it
returns a single-use "ticket" which must be used to exit the critical section:
```
   cst.Leave()
```
Note that the *Drain()* call blocks until the current ticket holder exits.

## Exec
The *Exec* interface overlays the os/exec package with support to launch commands and analyze errors.  See the unit test for usage.  A mocked version of the interface is available for unit testing.

## K8sSizeBytes

This is an *int64* data type that prints with a byte multiplier suffix.  For example,
```
  fmt.Printf("K8sSizeBytes=%s\n", util.SizeBytes(1024))
  fmt.Printf("K8sSizeBytes=%s\n", util.SizeBytes(1000))
  fmt.Printf("K8sSizeBytes=%s\n", util.SizeBytes(0))
```
would print
```
   SizeBytes=1Ki
   SizeBytes=1K
   SizeBytes=0
```
The *SizeBytesToString()* function provides the basic conversion used.

## Map key support
Support is provided to extract the *string* keys of a map, with optional sorting:
```
   var m = make(map[string]AnyType)
   // initialize m
   keys := util.StringKeys(m)
   sortedKeys := util.SortedStringKeys(m)
```
The sorted option is particularly useful in unit tests that want to predictably
traverse a map - Go maps have unpredictable ordering across process invocations.

## MsgList

The *MsgList* type is provided to help in adding messages to *models.TimestampedString* slices.
This data type is typically used by "Messages" fields in state properties.

*Insert()* accepts [fmt](https://golang.org/pkg/fmt/) like arguments to construct and
append a new message to the end of the slice.
The usage syntax uses a [Fluent](https://en.wikipedia.org/wiki/Fluent_interface) style:
```
  obj.Messages = util.NewMsgList(obj.Messages).Insert("hello %s", "world").ToModel()
```
The "ToModel()" call at the end ends the chain and returns an updated slice.
A *nil* initial value of obj.Messages is valid.  ToModel() will return the
array allocated internally to contain the messages, or nil if no Insert() calls
were made.

By default the current time is used for all messages.
One can create multiple messages with the same time stamp value by using
multiple *Insert()* calls in the chain, as follows:
```
  obj.Messages = util.NewMsgList(obj.Messages).
     Insert("msg %d", 1).
     Insert("msg %d", 2).
     ToModel()
```
It is also possible to specify a timestamp explicitly using *WithTimestamp()* as follows:
```
  obj.Messages = util.NewMsgList(obj.Messages).
    WithTimestamp(time.Now()).
    Insert("a message with the specified timestamp").
    ToModel()
```
The *WithTimestamp()* call applies to the *Insert()* operations that follow it.

The *NewTimestampedString()* subroutine provides the low level support to create a new
instance of *models.TimestampedString*.

## PanicLogger

This is meant to be used as an entry point for a goroutine, accepting a function as its argument.
It traps and logs the panic from the function:
```
  go PanicLogger(log, func(){ myFunc(args) })
```

## Queue

This provides queuing support for arbitrary element types.
At creation time a Queue must be declared to either be homogeneous or heterogeneous
with respect to the type of elements it may contain.
A homogeneous queue will panic if the wrong type of element is inserted.

Usage examples:
```
  q := NewQueue(&T{}) // create a homogeneous queue of *T elements
  q := NewQueue(nil) // create a heterogeneous queue

  // various insertion methods
  q.Append([]interface{}{&T{}, 1, "a")  // abstract slice mixed element types (heterogeneous queue only)
  q.Append([]interface{}{&T{}, &T{}})  // abstract slice with same element type (both kinds of queues)
  q.Add(&T{}) // single element (both kinds of queues)
  q.Add([]*T{&T{}, &T{}}) // slice of elements (homogeneous queue only)

  // access
  var p *T
  p = q.PeekHead().(*T) // requires a type assertion
  p = q.PopHead().(*T) // requires a type assertion

  q.PopN(q.Length()) // flush queue
```
See the unit test for more examples.

## Rounding Ticker

This provides support for a periodic timer that rounds its next scheduled time to a value provided
by the invoker. The effect is to consistently invoke the timer at the same wall-clock offset each period.

The *NewRoundingTicker()* subroutine is used to create such a timer.
The *Start()* method is used to start the timer and the *Stop()* method to terminate it.
A timer may be restarted.

## Request Middleware Counter
Provides methods to have a unique count associated with a http requests/responses

- NumberRequestMiddleware() will increment the counter and set it in the context.
- RequestNumberFromContext() will fetch the count that is set in a given context. It will return 0 if its unable to fetch the count.

## SizeBytes

This is an *int64* data type that prints with a byte multiplier suffix.  For example,
```
  fmt.Printf("SizeBytes=%s\n", util.SizeBytes(1024))
  fmt.Printf("SizeBytes=%s\n", util.SizeBytes(1000))
  fmt.Printf("SizeBytes=%s\n", util.SizeBytes(0))
```
would print
```
   SizeBytes=1KiB
   SizeBytes=1KB
   SizeBytes=0B
```
The *SizeBytesToString()* function provides the basic conversion used.

## Snapshot List

This provides support to create a list of snapshot data structures from a map,
with various sorting options:
```
  sMap := map[string]models.SnapshotData{...}
  unsorted := SnapshotMapToList(sMap)
  sortedOnSnapTime := SnapshotMapToListSorted(sMap, SnapshotSortOnSnapTime)
  sortedOnDeleteAfterTime := SnapshotMapToListSorted(sMap, SnapshotSortOnDeleteAfterTime)
```

## Status Extractor ResponseWriter

This provides an adaptor over an [http.ResponseWriter](https://golang.org/pkg/net/http/#ResponseWriter)
object to provide the following additional methods:

- StatusCode() returns the HTTP status code
- StatusSuccess() returns a boolean indicating if the status code is in the range *[200, 300)*

The *NewStatusExtractorResponseWriter* function is provided to create this object.
Typical use is for middleware to create such a wrapper to examine the status code on the
return path.

The returned object exposes the 3 optional HTTP/1.2 interfaces (CloseNotifier, Flusher and Hijacker) if they are *all present* - this seems to be the case of the ResponseWriter passed
to the middleware at runtime.
It does not pass on the HTTP/2 Pusher interface but Hijacker will always be exposed if present because it is needed by WebSockets.

## Tag List

Support to manipulate tag lists while preserving order.

```
tl := NewTagList(tagList)
v := tl.Get("tag")
tl.Set("tag", "v2")
tl.Set("newTag", "newValue")
tl.Delete("someTag")
tagList = tl.List()
for i:= 0; i < tl.Len(); i++ {
  k, v := tl.Idx(i)
}
```
## Time related utilities

The `Timestamp` data type can be used with the [go-flags](https://godoc.org/github.com/jessevdk/go-flags)
package and also provides a converter for use with [go-swagger](https://goswagger.io/).

The `TimeMaxUsefulUpperBound` function returns the maximum useful value to use in a Go [time.Time](https://golang.org/pkg/time/#Time) data type.
See [this Stackoverflow question](https://stackoverflow.com/questions/25065055/what-is-the-maximum-time-time-in-go)
for details.

The `DateTimeMaxUpperBound` function returns the maximum [strfmt.DateTime](https://github.com/go-openapi/strfmt)
value that may be safely processed by our testing framework (there are issues with cloning otherwise).

## ValidateIPAddress

ValidateIPAddress verifies the ipAddress is a valid textual representation of an IP address.

```
util.ValidateIPAddress("192.168.0.1") == true
```

## ValueType Parser

This provides support to parse a models.ValueType data structure.  It can be used in one of the following two patterns:
```
// explicit error returned
d, err := ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3.5h"})
i, err := ValueTypeInt(models.ValueType{Kind: "INT", Value: "12345"})
s, err := ValueTypeString(models.ValueType{Kind: "STRING", Value: "A String"})
x, err := ValueTypeString(models.ValueType{Kind: "SECRET", Value: "A Secret"})
p, err := ValueTypePercentage(models.ValueType{Kind: "PERCENTAGE", Value: "12.5%"})

// default value (of appropriate type) returned on error
var dv = ParseValueType(models.ValueType{Kind: "DURATION", Value: "90d"}, com.ValueTypeDuration, defDuration).(time.Duration)
var iv = ParseValueType(models.ValueType{Kind: "INT", Value: "12345"}, com.ValueTypeInt, defInt).(int64)
var sv = ParseValueType(models.ValueType{Kind: "STRING", Value: "A String"}, com.ValueTypeString, defString).(string)
var xv = ParseValueType(models.ValueType{Kind: "SECRET", Value: "A Secret"}, com.ValueTypeSecret, defSecret).(string)
var pv = ParseValueType(models.ValueType{Kind: "PERCENTAGE", Value: "12.5%"}, com.ValueTypePercentage, defPct).(float64)
```
Note that this parser interprets the duration suffix of "d" to mean 24 hours - time.ParseDuration() does not support "d" as a duration suffix.

## Worker

The *NewWorker* function creates a Worker to perform a task periodically on a goroutine when *Start()* is called.
The thread sleeps for a given interval when not doing work, but can be awoken from sleep with the *Notify()* method.
The task can abort itself by returning the **util.ErrWorkerAborted** error or
can be terminated from another thread with *Stop()*.