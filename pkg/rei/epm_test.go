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


package rei

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEPM(t *testing.T) {
	assert := assert.New(t)

	defer func() {
		os.RemoveAll("./testdir")
		os.RemoveAll("./global")
	}()

	// Not enabled => no directories made
	err := SetGlobalArena("./global/arena")
	assert.Equal(globalArena, "./global/arena")
	assert.NoError(err)
	fi, err := os.Stat(globalArena)
	assert.Error(err)
	arena := "testdir"
	epm := NewEPM(arena)
	epm = NewEPM(arena)
	assert.Equal(path.Join(globalArena, arena), epm.arenaPath)
	assert.Equal("global/arena/testdir", epm.arenaPath)
	fi, err = os.Stat(epm.arenaPath)
	assert.Error(err)

	globalArena = "" // reset

	// Enabled, new, no global arena, local testdir
	Enabled = true
	arena = "testdir"
	epm = NewEPM(arena)
	assert.NotNil(epm)
	assert.Equal(arena, epm.arenaPath)
	assert.Equal("testdir", epm.arenaPath)
	assert.NotNil(epm.cache)
	assert.False(epm.Enabled)

	fi, err = os.Stat(epm.arenaPath)
	assert.NoError(err)
	assert.True(fi.IsDir())

	// Enabled, new, specify global arena
	err = SetGlobalArena("./global/arena")
	assert.Equal(globalArena, "./global/arena")
	assert.NoError(err)
	epm = NewEPM(arena)
	assert.Equal(path.Join(globalArena, arena), epm.arenaPath)
	assert.Equal("global/arena/testdir", epm.arenaPath)
	assert.NotNil(epm.cache)
	assert.False(epm.Enabled)

	fi, err = os.Stat(epm.arenaPath)
	assert.NoError(err)
	assert.True(fi.IsDir())

	// SetProperty (UT like behavior)
	np := NewProperty()
	np.BoolValue = true
	assert.Empty(epm.cache)
	epm.SetProperty("p1", np)
	assert.NotEmpty(epm.cache)
	p, ok := epm.cache["p1"]
	assert.True(ok)
	assert.True(epm.GetBool("p1"))
	assert.False(epm.GetBool("p1")) // purged

	// Disable again (global arena already created)
	Enabled = false

	// create an empty property file
	// findProperty should not load it as long as effectively disabled
	err = ioutil.WriteFile(path.Join(epm.arenaPath, "p2"), []byte{}, 0600)
	assert.NoError(err)
	assert.False(Enabled)
	assert.False(epm.Enabled)
	assert.Empty(epm.cache)
	epm.loads = 0
	p = epm.findProperty("p2")
	assert.Nil(p)
	assert.Empty(epm.cache)
	Enabled = true
	p = epm.findProperty("p2")
	assert.Nil(p)
	assert.Empty(epm.cache)
	Enabled = false
	epm.Enabled = true
	p = epm.findProperty("p2")
	assert.Nil(p)
	assert.Empty(epm.cache)
	assert.False(epm.GetBool("bPt"))
	assert.Equal(0, epm.GetInt("iPt"))
	assert.Equal(float64(0), epm.GetFloat("fPt"))
	assert.Equal("", epm.GetString("sPt"))
	assert.Equal(0, epm.loads)

	// load the property from file
	Enabled = true
	epm.Enabled = true
	epm.loads = 0
	p = epm.findProperty("p2")
	assert.NotNil(p)
	assert.True(p.loaded)
	assert.Empty(epm.cache) // purged
	assert.Equal(0, p.NumUses)
	assert.Equal(1, epm.loads)

	// can load, load failed
	epm.loads = 0
	p = epm.findProperty("p2")
	assert.Nil(p)
	assert.Empty(epm.cache)
	assert.Equal(1, epm.loads)

	// create a property file for 2 uses
	err = createPropFile(path.Join(epm.arenaPath, "p3"), &Property{NumUses: 2})
	assert.NoError(err)
	p = epm.findProperty("p3")
	assert.NotNil(p)
	assert.Equal(1, p.NumUses, "%v", p)
	assert.True(p.loaded)
	assert.NotEmpty(epm.cache)
	p = epm.findProperty("p3")
	assert.NotNil(p)
	assert.Equal(0, p.NumUses)
	assert.True(p.loaded)
	assert.Empty(epm.cache)

	// create different typed property files
	err = createPropFile(path.Join(epm.arenaPath, "bPt"), &Property{BoolValue: true, NumUses: 2})
	assert.NoError(err)
	assert.True(epm.GetBool("bPt"))
	err = epm.ErrOnBool("bPt")
	assert.Error(err)
	assert.Regexp("injecting error testdir/bPt", err)
	assert.Empty(epm.cache)
	assert.False(epm.GetBool("bPt"))

	err = createPropFile(path.Join(epm.arenaPath, "bPf"), &Property{BoolValue: false, NumUses: 3})
	assert.NoError(err)
	assert.False(epm.GetBool("bPf"))
	err = epm.ErrOnBool("bPf")
	assert.NoError(err)
	p = epm.findProperty("bPf")
	assert.False(p.BoolValue)
	assert.Empty(epm.cache)

	err = createPropFile(path.Join(epm.arenaPath, "iPt"), &Property{IntValue: 1})
	assert.NoError(err)
	assert.Equal(1, epm.GetInt("iPt"))
	assert.Empty(epm.cache)
	assert.Equal(0, epm.GetInt("iPt"))

	err = createPropFile(path.Join(epm.arenaPath, "fPt"), &Property{FloatValue: 3.142})
	assert.NoError(err)
	assert.Equal(float64(3.142), epm.GetFloat("fPt"))
	assert.Empty(epm.cache)
	assert.Equal(float64(0), epm.GetFloat("fPt"))

	err = createPropFile(path.Join(epm.arenaPath, "sPt"), &Property{StringValue: "string value"})
	assert.NoError(err)
	assert.Equal("string value", epm.GetString("sPt"))
	assert.Empty(epm.cache)
	assert.Equal("", epm.GetString("sPt"))
}

func createPropFile(fp string, prop *Property) error {
	b, err := json.Marshal(prop)
	if err == nil {
		err = ioutil.WriteFile(fp, b, 0600)
	}
	return err
}
