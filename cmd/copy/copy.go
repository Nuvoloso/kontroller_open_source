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
	"flag"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"time"
)

var resultsFileName string
var argFileName string
var argExample bool

// How long with no progress before aborting
var timeoutValue = 430 * time.Second

// Number of second between progress updates
var progressTick int32 = 13

type copyError struct {
	Description string
	Fatal       bool
	full        bool
	mux         sync.Mutex
}

func (ce *copyError) set(ErrStr string, fatal bool) {
	// Lock required so that first one in wins
	ce.mux.Lock()
	if !ce.full {
		ce.Description = ErrStr
		ce.Fatal = fatal
		ce.full = true
	}
	ce.mux.Unlock()
}

func (ce *copyError) isSet() bool {
	return ce.full
}

type copyResponse struct {
	Stats stats.Stats // statistics regarding this copy
	Error copyError   // Error that occurred, if any
	mux   sync.Mutex  // write Mutex
}

var abortMe func(resp *copyResponse)

func newCopyResponse() *copyResponse {
	r := copyResponse{}
	r.Stats.Init()

	return &r
}

// writeFile Writes the copy response to a file
func (cr *copyResponse) writeFile(fn string) error {
	var err error
	var f *os.File

	if fn != "" {
		cr.mux.Lock()
		defer cr.mux.Unlock()
		f, err = os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err == nil {
			b, _ := json.Marshal(cr)
			buf := bytes.NewBuffer(b)
			io.Copy(f, buf)
			f.Close()
		}
	}
	return err
}

// progress will produce regular progress reports and abort on a lack of progress
func progress(progressFileName string, resp *copyResponse) {
	go func() {
		var prevProgress int64
		progressTime := time.Now()

		ticker := time.NewTicker(time.Duration(progressTick) * time.Second)

		for range ticker.C {
			if progressFileName != "" {
				resp.writeFile(progressFileName)
			}

			// If not making progress, time out
			currentProgress := resp.Stats.Get(endpoint.StatNameProgress)
			if currentProgress != prevProgress {
				prevProgress = currentProgress
				progressTime = time.Now()
			} else {
				if time.Since(progressTime) >= timeoutValue {
					ticker.Stop()
					// if an error hasn't been set, say "lack of progress"
					resp.Error.set("Aborted due to a lack of progress", true)
					abortMe(resp)
				}
			}
		}
	}()
}

// killFunc is a hard shutdown
func killFunc(resp *copyResponse) {
	if err := resp.writeFile(resultsFileName); err != nil {
		os.Exit(1)
	}
	os.Exit(0) // Results file is good
}

func init() {
	flag.StringVar(&resultsFileName, "results", "", "Results Data File Name")
	flag.BoolVar(&argExample, "example", false, "Give an example argument file")
	flag.StringVar(&argFileName, "args", "args.json", "File Containing the Arguments for the Copy")
	abortMe = killFunc
}

func worker(id uint,
	work chan *metadata.ChunkInfo,
	wg *sync.WaitGroup,
	resp *copyResponse,
	src endpoint.EndPoint,
	dst endpoint.EndPoint) {

	globError := &resp.Error

	defer wg.Done()

	myName := fmt.Sprintf("Worker%dState", id)

	buf := make([]byte, metadata.ObjSize)

	for ci := range work {
		if ci.Dirty {
			resp.Stats.Set(myName, 1)
			e := src.FetchAt(ci.Offset, ci.Hash, buf)
			if e != nil {
				globError.set("FetchAt: "+e.Error(), true)
				break
			}
			resp.Stats.Set(myName, 2)
			e = dst.FillAt(ci.Offset, ci.Hash, buf)
			if e != nil {
				globError.set("FillAt: "+e.Error(), true)
				break
			}
			resp.Stats.Set(myName, 3)
		} else {
			resp.Stats.Set(myName, 4)
			e := dst.SkipAt(ci.Offset, ci.Hash)
			if e != nil {
				globError.set("SkipAt: "+e.Error(), true)
				break
			}
			resp.Stats.Set(myName, 5)
		}
	}
	if globError.isSet() == false {
		resp.Stats.Set(myName, 6)
	} else {
		resp.Stats.Set(myName, 7)
	}
}

func doCopy(args *endpoint.CopyArgs) *copyResponse {
	resp := newCopyResponse()
	resp.Stats.Set(endpoint.StatNameDestBytesWritten, 0)
	resp.Stats.Set(endpoint.StatNameDiffBytesChanged, 0)
	resp.Stats.Set(endpoint.StatNameDiffBytesUnchanged, 0)
	resp.Stats.Set(endpoint.StatNameProgress, 0)
	resp.Stats.Set("State", 0)

	progress(args.ProgressFileName, resp)

	if args.NumThreads == 0 {
		args.NumThreads = 1
	}

	resp.Stats.Set("State", 20)
	ea := &endpoint.Arg{
		Purpose: "Source",
		Type:    args.SrcType,
		Args:    args.SrcArgs,
	}
	src, err := endpoint.SetupEndpoint(ea, &resp.Stats, args.NumThreads)
	if err != nil {
		resp.Error.set("Set up source endpoint: "+err.Error(), true)
		return resp
	}

	resp.Stats.Set("State", 30)
	ea = &endpoint.Arg{
		Purpose: "Destination",
		Type:    args.DstType,
		Args:    args.DstArgs,
	}
	dst, err := endpoint.SetupEndpoint(ea, &resp.Stats, args.NumThreads)
	if err != nil {
		resp.Error.set("Set up destination endpoint: "+err.Error(), true)
		return resp
	}

	ea = nil // free

	resp.Stats.Set("Size", src.GetLunSize())

	workChan := make(chan *metadata.ChunkInfo, args.NumThreads*4)
	var workerwg sync.WaitGroup

	resp.Stats.Set("State", 40)

	for i := args.NumThreads; i != 0; i-- {
		workerwg.Add(1)
		go worker(i, workChan, &workerwg, resp, src, dst)
	}

	start := time.Now()

	resp.Stats.Set("State", 50)

	diffErr := src.Differ(workChan, resp.Error.isSet)
	if diffErr != nil {
		resp.Error.set("Differ: "+diffErr.Error(), true)
	}

	resp.Stats.Set("State", 60)

	workerwg.Wait()

	resp.Stats.Set("State", 100)

	e := src.Done(!resp.Error.isSet() && diffErr == nil)
	if e != nil {
		resp.Error.set("Source: "+e.Error(), true)
	}

	resp.Stats.Set("State", 200)

	e = dst.Done(!resp.Error.isSet() && diffErr == nil)
	if e != nil {
		resp.Error.set("Destination: "+e.Error(), true)
	}

	resp.Stats.Set("State", 300)

	elapsed := time.Since(start)

	resp.Stats.Set("CopyDurationSec", int64(elapsed.Seconds()))

	return resp
}

func main() {
	flag.Parse()

	// Dump out a empty JSON argument example
	if argExample == true {
		b, err := json.Marshal(&endpoint.CopyArgs{})
		if err == nil {
			buf := bytes.NewBuffer(b)
			io.Copy(os.Stdout, buf)
			fmt.Println("")
		}
		return
	}

	f, err := os.Open(argFileName)
	if err != nil {
		resp := newCopyResponse()
		resp.Error.set("argument file: "+argFileName+": "+err.Error(), true)
		_ = resp.writeFile(resultsFileName)
		return
	}
	defer f.Close()

	argsBytes, err := ioutil.ReadAll(f)
	if err != nil {
		resp := newCopyResponse()
		resp.Error.set("read argument file: "+argFileName+": "+err.Error(), true)
		_ = resp.writeFile(resultsFileName)
		return
	}

	var args endpoint.CopyArgs

	err = json.Unmarshal(argsBytes, &args)
	if err != nil {
		resp := newCopyResponse()
		resp.Error.set("unmarshal argument file: "+argFileName+": "+err.Error(), true)
		_ = resp.writeFile(resultsFileName)
		return
	}

	resp := doCopy(&args)
	if err = resp.writeFile(resultsFileName); err != nil {
		os.Exit(1)
	}
}
