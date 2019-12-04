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
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

type fakeCopyStats struct {
	BytesWritten int64
}

type fakeCopyError struct {
	Description string
	Fatal       bool
}

type fakeCopyResults struct {
	Stats fakeCopyStats
	Error fakeCopyError
}

type bogusResults struct {
	nothing string
}

// writeFakeCopyResults Writes a fake copy response to a file
func writeFakeCopyResults(fn string, cr *fakeCopyResults) error {
	var err error
	var f *os.File

	f, err = os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(cr)
	buf := bytes.NewBuffer(b)
	io.Copy(f, buf)
	f.Close()

	return nil
}

func writeBadResults(fn string) error {
	var err error
	var f *os.File

	f, err = os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}

	f.Write([]byte("This is not even Close to JSON"))
	f.Close()

	return nil
}

func TestSpawnCopy(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	ca := &ControllerArgs{Log: tl.Logger()}

	truePath, err := exec.LookPath("true")
	assert.NoError(err)

	falsePath, err := exec.LookPath("false")
	assert.NoError(err)

	ops, err := NewController(ca)
	assert.NoError(err)
	assert.NotNil(ops)
	c, ok := ops.(*Controller)
	assert.True(ok)
	assert.NotNil(c)

	cArgs := endpoint.CopyArgs{
		SrcType: "Fake",
		DstType: "Fake",
	}
	ctx := context.Background()
	bRes := SnapshotBackupResult{}
	rRes := SnapshotRestoreResult{}

	// Fail creation of args file by providing a bad ID
	err = c.SpawnCopy(ctx, "id/has/slashes", &cArgs, &bRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.Regexp("copy: create argument file:", err)

	// Good Executable Positive response
	copyLocation = truePath
	err = c.SpawnCopy(ctx, "true", &cArgs, &bRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.Regexp("copy: results file not found:", err)

	// Good Executable Negative response, rei block
	copyLocation = falsePath
	c.rei.SetProperty("copy-block-on-error", &rei.Property{BoolValue: true})
	err = c.SpawnCopy(ctx, "false", &cArgs, &rRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.Regexp("copy: no results:", err)

	// Bad Executable
	copyLocation = "Bogus"
	err = c.SpawnCopy(ctx, "Bogus", &cArgs, &bRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.Regexp("copy: run:", err)

	fakeRes := &fakeCopyResults{
		Error: fakeCopyError{
			Description: "FakeError",
			Fatal:       false,
		},
	}

	err = writeFakeCopyResults("/tmp/results-fake", fakeRes)
	assert.Nil(err)

	copyLocation = truePath
	err = c.SpawnCopy(ctx, "fake", &cArgs, &bRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.True(bRes.Error.Description != "")
	assert.False(bRes.Error.Fatal)

	err = writeFakeCopyResults("/tmp/results-fake", fakeRes)
	assert.Nil(err)

	copyLocation = truePath
	err = c.SpawnCopy(ctx, "fake", &cArgs, &rRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.True(bRes.Error.Description != "")
	assert.False(bRes.Error.Fatal)

	fakeRes.Error.Fatal = true
	err = writeFakeCopyResults("/tmp/results-fake", fakeRes)
	assert.Nil(err)
	copyLocation = truePath
	err = c.SpawnCopy(ctx, "fake", &cArgs, &bRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.True(bRes.Error.Description != "")
	assert.True(bRes.Error.Fatal)

	err = writeFakeCopyResults("/tmp/results-fake", fakeRes)
	assert.Nil(err)
	copyLocation = truePath
	err = c.SpawnCopy(ctx, "fake", &cArgs, &rRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.True(bRes.Error.Description != "")
	assert.True(bRes.Error.Fatal)

	err = writeFakeCopyResults("/tmp/results-fake", fakeRes)
	assert.Nil(err)
	br := bogusResults{}
	copyLocation = truePath
	err = c.SpawnCopy(ctx, "fake", &cArgs, &br)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.True(bRes.Error.Description != "")
	assert.True(bRes.Error.Fatal)

	err = writeBadResults("/tmp/results-fake")
	assert.Nil(err)
	copyLocation = truePath
	err = c.SpawnCopy(ctx, "fake", &cArgs, &rRes)
	assert.NotNil(err)
	t.Logf("err: %v", err)
	assert.True(bRes.Error.Description != "")
	assert.True(bRes.Error.Fatal)
}
