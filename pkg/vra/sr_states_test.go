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


package vra

import (
	"testing"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestStorageRequestState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	rss := TerminalStorageRequestStates()
	assert.NotNil(rss)
	assert.NotEmpty(rss)
	assert.Equal(rss, terminalStorageRequestStates)
	for _, rs := range rss {
		assert.True(rs != "")
		assert.True(StorageRequestStateIsTerminated(rs))
		assert.False(StorageRequestStateIsTerminated(rs + "x"))
	}
}

func TestStorageRequestAnimatorProcessStates(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	for op, ap := range initialSRProcessOfOperation {
		apOut := GetSRProcess(com.StgReqStateNew, op)
		assert.Equal(ap, apOut)
	}

	apOut := GetSRProcess(com.StgReqStateNew, "garbage op")
	assert.Equal(ApUnknown, apOut)

	for state, ap := range srStateToProcess {
		apOut = GetSRProcess(state, "anythingelse")
		if state == com.StgReqStateNew {
			assert.Equal(ApUnknown, apOut)
		} else {
			assert.Equal(ap, apOut)
		}
	}

	assert.Panics(func() { GetSRProcess("garbage", "truck") })
}
