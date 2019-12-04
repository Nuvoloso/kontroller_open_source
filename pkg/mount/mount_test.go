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


package mount

import (
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	var tB, tA time.Time

	// MounterArgs
	ma := &MounterArgs{}
	err := ma.Validate()
	assert.Equal(ErrInvalidArgs, err)

	ma.Log = tl.Logger()
	err = ma.Validate()
	assert.NoError(err)

	// FilesystemMountArgs
	badFMAs := []FilesystemMountArgs{
		{},
		{Source: "source"},
		{Target: "target"},
	}
	for i, fma := range badFMAs {
		err = fma.Validate()
		assert.Equal(ErrInvalidArgs, err, "fma[%d]", i)
	}
	fma := &FilesystemMountArgs{
		Source: "source",
		Target: "target",
	}
	tB = time.Now()
	err = fma.Validate()
	tA = time.Now()
	assert.NoError(err)
	assert.WithinDuration(tA.Add(DefaultCommandTimeout), fma.Deadline, tA.Sub(tB)) // deadline defaulted

	fma.Deadline = tB
	err = fma.Validate()
	assert.NoError(err)
	assert.True(tB.Equal(fma.Deadline)) // deadline as specified

	// FilesystemUnmountArgs
	fua := &FilesystemUnmountArgs{}
	err = fua.Validate()
	assert.Equal(ErrInvalidArgs, err)

	fua.Target = "target"
	tB = time.Now()
	err = fua.Validate()
	tA = time.Now()
	assert.NoError(err)
	assert.WithinDuration(tA.Add(DefaultCommandTimeout), fua.Deadline, tA.Sub(tB)) // deadline defaulted

	fua.Deadline = tB
	err = fua.Validate()
	assert.NoError(err)
	assert.True(tB.Equal(fua.Deadline)) // deadline as specified

	// FilesystemFreezeArgs
	ffa := &FilesystemFreezeArgs{}
	err = ffa.Validate()
	assert.Equal(ErrInvalidArgs, err)

	ffa.Target = "target"
	tB = time.Now()
	err = ffa.Validate()
	tA = time.Now()
	assert.NoError(err)
	assert.WithinDuration(tA.Add(DefaultCommandTimeout), ffa.Deadline, tA.Sub(tB)) // deadline defaulted

	ffa.Deadline = tB
	err = ffa.Validate()
	assert.NoError(err)
	assert.True(tB.Equal(ffa.Deadline)) // deadline as specified
}
