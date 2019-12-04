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


package pstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/endpoint"
)

// CopyStats common struct that contains copy stats
type CopyStats struct {
	BytesTransferred int64 // The number of bytes written to the destination
	SrcReadCount     int64 // The number of reads from the source
	BytesChanged     int64 // The number of bytes found to be changed on the source
	BytesUnchanged   int64 // The number of bytes found to be unchanged on the source
	Size             int64 // The size of the file we are copying
	OffsetProgress   int64 // How far along we are currently
}

// progressReport constructs a progress report from the stats
func (cs *CopyStats) progressReport() *CopyProgressReport {
	var percentComplete float64
	if cs.Size > 0 {
		var num int64
		if cs.OffsetProgress > 0 {
			num = cs.OffsetProgress
		} else if cs.BytesTransferred > 0 {
			num = cs.BytesTransferred
		}
		percentComplete = float64(num) * 100 / float64(cs.Size)
	}
	return &CopyProgressReport{
		TransferredBytes: cs.BytesTransferred,
		TotalBytes:       cs.Size,
		OffsetBytes:      cs.OffsetProgress,
		PercentComplete:  percentComplete,
		SrcReadCount:     cs.SrcReadCount,
	}
}

// CopyError common error
type CopyError struct {
	Description string
	Fatal       bool
}

// copyFatalError constructs an ErrorDesc
func copyFatalError(action string, err error) error {
	return NewFatalError(fmt.Sprintf("copy: %s: %s", action, err.Error()))
}

// SpawnCopy spawns the copy executable with the appropriate arguments
func (c *Controller) SpawnCopy(ctx context.Context, ID string, args *endpoint.CopyArgs, res interface{}) error {
	// construct temporary file names
	resFileName := "/tmp/results-" + ID
	argFileName := "/tmp/args-" + ID
	defer os.Remove(argFileName)
	defer os.Remove(resFileName)
	// Write copy args to a file
	f, err := os.Create(argFileName)
	if err == nil {
		// encode arguments
		b, _ := json.Marshal(args)
		buf := bytes.NewBuffer(b)
		_, err = io.Copy(f, buf)
		f.Close()
	}
	if err != nil {
		return copyFatalError("create argument file", err)
	}
	// Start the copy
	cmd := exec.CommandContext(ctx, copyLocation, "-args", argFileName, "-results", resFileName)
	output, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		return copyFatalError("run", err)
	}
	var stdOutBuf bytes.Buffer
	io.Copy(&stdOutBuf, output)
	// Wait for the copy to finish
	err = cmd.Wait()
	if err != nil {
		rc := cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
		c.Log.Errorf("%s command failed rc=%d af=[%s] rf=[%s]", ID, rc, argFileName, resFileName)
		for c.rei.GetBool("copy-block-on-error") {
			time.Sleep(1 * time.Second)
		}
		// These errors only come back when it was not able to write the results
		return copyFatalError("no results", err)
	}
	// decode results
	return c.intOps.ReadResult(resFileName, res)
}

// ReadResult reads and unmarshals the result JSON from a file
func (c *Controller) ReadResult(resFileName string, res interface{}) error {
	f, err := os.Open(resFileName)
	if err != nil {
		return copyFatalError("results file not found", err)
	}
	defer f.Close()
	resBytes, err := ioutil.ReadAll(f)
	if err == nil {
		err = json.Unmarshal(resBytes, res)
	}
	if err != nil {
		return copyFatalError("poorly formatted results", err)
	}

	// Translate the copy results error into an ErrorDesc
	var ce *CopyError
	switch v := res.(type) {
	case *SnapshotBackupResult:
		ce = &v.Error
	case *SnapshotRestoreResult:
		ce = &v.Error
	default:
		return copyFatalError("failed", fmt.Errorf("result type not expected: %T", res))
	}
	if ce != nil && ce.Description != "" {
		c.Log.Errorf("copy command failed Description=%s rf=[%s]", ce.Description, resFileName)
		return &errorDesc{
			description: fmt.Sprintf("copy failed: %s", ce.Description),
			retryable:   !ce.Fatal,
		}
	}
	return nil
}
