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


package csi

import (
	"context"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestControllerHandlerMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// invalid args
	invalidTCs := []ControllerHandlerArgs{
		ControllerHandlerArgs{},
		ControllerHandlerArgs{Ops: &fakeCHO{}},
		ControllerHandlerArgs{Ops: &fakeCHO{}, HandlerArgs: HandlerArgs{Log: tl.Logger(), Socket: "foo", Version: ""}},
		ControllerHandlerArgs{Ops: &fakeCHO{}, HandlerArgs: HandlerArgs{Socket: "foo", Version: "0.2.3"}},
		ControllerHandlerArgs{Ops: &fakeCHO{}, HandlerArgs: HandlerArgs{Log: tl.Logger(), Version: "0.2.3"}},
	}
	for i, tc := range invalidTCs {
		assert.Error(tc.Validate(), "case %d", i)
		ch, err := NewControllerHandler(&tc)
		assert.Error(err, "case %d", i)
		assert.Nil(ch, "case %d", i)
	}

	// valid args
	cha := &ControllerHandlerArgs{
		Ops: &fakeCHO{},
		HandlerArgs: HandlerArgs{
			Socket:  "./foo",
			Log:     tl.Logger(),
			Version: "0.6.2",
		},
	}
	assert.NoError(cha.Validate())

	// create succeeds
	ch, err := NewControllerHandler(cha)
	assert.NoError(err)
	assert.NotNil(ch)

	h, ok := ch.(*controllerHandler)
	assert.True(ok)
	assert.NotNil(h)

	// Start is currently a no-op
	ch.Start()

	// Stop is currently a no-op
	ch.Stop()
}

type fakeCHO struct {
	RetCVID    string
	RetCVIDErr error
	InDVID     string
	RetDVErr   error
	RetIsReady bool
} // TBD: add methods as they get defined

func (f *fakeCHO) CreateVolumeID(ctx context.Context) (string, error) {
	return f.RetCVID, f.RetCVIDErr
}

func (f *fakeCHO) DeleteVolume(ctx context.Context, volID string) error {
	f.InDVID = volID
	return f.RetDVErr
}

func (f *fakeCHO) IsReady() bool {
	return f.RetIsReady
}
