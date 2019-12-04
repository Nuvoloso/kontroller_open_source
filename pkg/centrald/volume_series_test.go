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


package centrald

import (
	"regexp"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestMountModes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	smm := app.SupportedMountModes()
	assert.NotNil(smm)
	assert.NotEmpty(smm)
	assert.Equal(smm, supportedMountModes)
	assert.False(app.ValidateMountMode(""))
	for _, m := range smm {
		assert.True(m != "")
		assert.True(app.ValidateMountMode(m))
		assert.False(app.ValidateMountMode(m + "x"))
	}

	// check that all state constants are in the supported array
	bc, err := testutils.FetchBasicConstantsFromGoFile("../common/consts.go")
	assert.NoError(err)
	sMap := bc.GetStringConstants(regexp.MustCompile("^VolMountMode[A-Z]"))
	for n, v := range sMap {
		assert.True(app.ValidateMountMode(v), "Missing VolMountMode constant %s ('%s')", n, v)
	}
}

func TestMountStates(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sms := app.SupportedMountStates()
	assert.NotNil(sms)
	assert.NotEmpty(sms)
	assert.Equal(sms, supportedMountStates)
	assert.False(app.ValidateMountState(""))
	for _, ms := range sms {
		assert.True(ms != "")
		assert.True(app.ValidateMountState(ms))
		assert.False(app.ValidateMountState(ms + "x"))
	}

	// check that all state constants are in the supported array
	bc, err := testutils.FetchBasicConstantsFromGoFile("../common/consts.go")
	assert.NoError(err)
	sMap := bc.GetStringConstants(regexp.MustCompile("^VolMountState[A-Z]"))
	for n, v := range sMap {
		if v != common.VolMountStateUnmounted {
			assert.True(app.ValidateMountState(v), "Missing VolMountState constant %s ('%s')", n, v)
		}
	}
}

func TestVolumeSeriesStates(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sms := app.SupportedVolumeSeriesStates()
	assert.NotNil(sms)
	assert.NotEmpty(sms)
	assert.Equal(sms, supportedVolumeSeriesStates)
	assert.False(app.ValidateVolumeSeriesState(""))
	for _, ms := range sms {
		assert.True(ms != "")
		assert.True(app.ValidateVolumeSeriesState(ms))
		assert.False(app.ValidateVolumeSeriesState(ms + "x"))
	}

	// check that all  state constants are in the supported array
	bc, err := testutils.FetchBasicConstantsFromGoFile("../common/consts.go")
	assert.NoError(err)
	sMap := bc.GetStringConstants(regexp.MustCompile("^VolState[A-Z]"))
	for n, v := range sMap {
		assert.True(app.ValidateVolumeSeriesState(v), "Missing VolState constant %s ('%s')", n, v)
	}
}
