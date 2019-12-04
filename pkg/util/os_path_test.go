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


package util

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestWaitForPath(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// validate fails in waitforpath
	ctx := context.Background()
	wfdArgs := &WaitForPathArgs{}
	ret, err := WaitForPath(ctx, wfdArgs)
	assert.NotNil(err)
	assert.Equal(time.Duration(0), wfdArgs.RetryInterval)

	// validate set retry interval
	wfdArgs.Path = "/some/path/to/dir"
	err = wfdArgs.Validate()
	assert.Nil(err)
	assert.Equal(OSStatRetryDuration, wfdArgs.RetryInterval)

	// validate does not overwrite retry interval
	wfdArgs.RetryInterval = time.Duration(time.Millisecond * 5)
	err = wfdArgs.Validate()
	assert.Nil(err)
	assert.Equal(time.Duration(time.Millisecond*5), wfdArgs.RetryInterval)

	// finds the path on first try
	ctx = context.Background()
	dirPath := "./testPath"
	os.Mkdir(dirPath, 0700)
	wfdArgs = &WaitForPathArgs{
		Path:          dirPath,
		RetryInterval: time.Duration(time.Millisecond * 1),
	}
	ret, err = WaitForPath(ctx, wfdArgs)
	assert.Nil(err)
	assert.NotNil(ret.FInfo)
	assert.False(ret.Retry)
	os.Remove(dirPath)

	// created while executing
	ctx = context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*50)
	defer cancel()
	go func() {
		time.Sleep(4 * time.Millisecond)
		os.Mkdir(dirPath, 0700)
	}()
	defer os.Remove(dirPath)

	wfdArgs = &WaitForPathArgs{
		Path:          dirPath,
		RetryInterval: time.Duration(time.Millisecond * 1),
	}
	ret, err = WaitForPath(ctx, wfdArgs)
	assert.Nil(err)
	assert.NotNil(ret.FInfo)
	os.Remove(dirPath)
	assert.True(ret.Retry)

	// ctx expired
	ctx = context.Background()
	ctx, cancel = context.WithTimeout(ctx, time.Millisecond*50)
	defer cancel()
	ret, err = WaitForPath(ctx, wfdArgs)
	assert.NotNil(err)
	assert.Nil(ret)

	// stat error not NotExist
	ctx = context.Background()
	ctx, cancel = context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	dirPath = "./foo"
	os.Mkdir(dirPath, 0)
	defer os.Remove(dirPath)
	wfdArgs.Path = dirPath + "/bar"
	ret, err = WaitForPath(ctx, wfdArgs)
	assert.NotNil(err)
	assert.Regexp("permission denied", err)
	assert.Nil(ret)
}
