// Copyright 2019 Tad Lebeck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package main

import (
	"bytes"
	"encoding/json"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"io"
	"os"
	"testing"
)

func TestNoFail(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType:          "Fake",
		DstType:          "Fake",
		ProgressFileName: "/tmp/progress",
	}

	res := doCopy(&ca)
	if res.Error.Description != "" {
		t.Error("Failed when should have succeeded")
	}

	res.writeFile("/tmp/junk")
	os.Remove("/tmp/junk")
}

func TestFailDiff(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailDiffer: true,
			},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
	}

	res := doCopy(&ca)
	if res.Error.Description != "Differ: Forced Error" {
		t.Error("Differ should have Failed Got: " + res.Error.Description)
	}
}

func TestFailDstConfig(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailConfig: true,
			},
		},
	}

	res := doCopy(&ca)
	if res.Error.Description != "Set up destination endpoint: Forced Error" {
		t.Error("Config should have Failed")
	}
}

func TestFailFetch(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailFetch: true,
			},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
	}

	res := doCopy(&ca)
	if res.Error.Description != "FetchAt: Forced Error" {
		t.Error("Fetch should have Failed Got: " + res.Error.Description)
	}
	if res.Error.Fatal != true {
		t.Error("Error should be fatal")
	}
}

func TestFailFill(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailFill: true,
			},
		},
	}

	res := doCopy(&ca)
	if res.Error.Description != "FillAt: Forced Error" {
		t.Error("Fill should have Failed Got: " + res.Error.Description)
	}
	if res.Error.Fatal != true {
		t.Error("Error should be fatal")
	}
}

func TestFailSkip(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailSkip: true,
			},
		},
	}

	res := doCopy(&ca)
	if res.Error.Description != "SkipAt: Forced Error" {
		t.Error("Skip should have Failed Got: " + res.Error.Description)
	}
	if res.Error.Fatal != true {
		t.Error("Error should be fatal")
	}
}

func TestFailDoneSrc(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailDone: true,
			},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
	}

	res := doCopy(&ca)
	if res.Error.Description != "Source: Forced Error" {
		t.Error("Done should have Failed Got: " + res.Error.Description)
	}
	if res.Error.Fatal != true {
		t.Error("Error should be fatal")
	}
}

func TestFailDoneDst(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FailDone: true,
			},
		},
		NumThreads: 1,
	}

	res := doCopy(&ca)
	if res.Error.Description != "Destination: Forced Error" {
		t.Error("Done Destination should have Failed Got: " + res.Error.Description)
	}
	if res.Error.Fatal != true {
		t.Error("Error should be fatal")
	}
}

func TestFailSetupEndpoints(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Bogus",
		DstType: "Bogus",
	}

	res := doCopy(&ca)
	if res.Error.Description != "Set up source endpoint: Unsupported Endpoint Type: Bogus" {
		t.Error("Endpoint Config have Failed Got: " + res.Error.Description)
	}
	if res.Error.Fatal == false {
		t.Error("Error should be fatal")
	}
}

func TestMainProg(t *testing.T) {
	ca := endpoint.CopyArgs{
		SrcType: "Fake",
		DstType: "Fake",
	}
	f, err := os.OpenFile("/tmp/argFile", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		t.Error("Create of argfile failed")
	}

	b, _ := json.Marshal(ca)
	buf := bytes.NewBuffer(b)
	io.Copy(f, buf)
	f.Close()
	defer os.Remove("/tmp/argFile")

	os.Args = []string{"copy", "--args", "/tmp/argFile", "--results", "/tmp/resFile"}
	main()
	os.Remove("/tmp/resFile")
}

func TestMissingArgFile(t *testing.T) {
	os.Args = []string{"copy", "--args", "/tmp/missing", "--results", "/tmp/resFile"}
	main()
	os.Remove("/tmp/resFile")
}

func TestEmptyArgFile(t *testing.T) {
	f, err := os.OpenFile("/tmp/argFile", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		t.Error("Create of argfile failed")
	}

	f.Close()
	defer os.Remove("/tmp/argFile")

	os.Args = []string{"copy", "--args", "/tmp/argFile", "--results", "/tmp/resFile"}
	main()
	os.Remove("/tmp/resFile")
}

func TestExample(t *testing.T) {
	f, err := os.Create("/tmp/example")
	if err != nil {
		t.Error("Can't create example file")
	}
	defer os.Remove("/tmp/example")
	oldStdout := os.Stdout
	os.Stdout = f
	os.Args = []string{"copy", "-example"}
	main()
	os.Stdout = oldStdout
	f.Close()
}

var aborted bool

func testAbortFunc(resp *copyResponse) {
	resp.Error.set("testAbortFunc: Timed Out", true)
	aborted = true
}

func TestTimeout(t *testing.T) {
	ca := endpoint.CopyArgs{
		ProgressFileName: "/tmp/progress",
		SrcType:          "Fake",
		SrcArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{
				FetchDelay: 5,
			},
		},
		DstType: "Fake",
		DstArgs: endpoint.AllArgs{
			Fake: endpoint.FakeArgs{},
		},
	}

	abortMe = testAbortFunc
	timeoutValue = 1
	progressTick = 1
	aborted = false
	doCopy(&ca)

	if aborted != true {
		t.Error("Should have timed out")
	}
}
