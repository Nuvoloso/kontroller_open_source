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
	"fmt"
	"os"
	"time"
)

// contants for os path related functions
const (
	OSStatRetryDuration = time.Duration(time.Millisecond * 10)
)

// WaitForPathArgs describes args used to wait for a directory to be present
type WaitForPathArgs struct {
	RetryInterval time.Duration
	Path          string
}

// WaitForPathRetVal describes the return value when checking if a directory is present
type WaitForPathRetVal struct {
	FInfo     os.FileInfo
	TimeTaken time.Duration
	Retry     bool
}

// Validate validates the arguments provided to WaitForPath
func (wfp *WaitForPathArgs) Validate() error {
	if wfp.Path == "" {
		return fmt.Errorf("empty dirPath")
	}
	if wfp.RetryInterval == 0 {
		wfp.RetryInterval = OSStatRetryDuration
	}
	return nil
}

// WaitForPath take a directory path and waits for it to be present or returns an error
func WaitForPath(ctx context.Context, args *WaitForPathArgs) (*WaitForPathRetVal, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}
	timeStart := time.Now()
	retry := false
	for {
		info, err := os.Stat(args.Path)
		if err == nil {
			return &WaitForPathRetVal{
				FInfo:     info,
				TimeTaken: time.Now().Sub(timeStart),
				Retry:     retry,
			}, nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("WaitForPath context expired")
		case <-time.After(args.RetryInterval):
			// wait
		}
		retry = true
	}
}
