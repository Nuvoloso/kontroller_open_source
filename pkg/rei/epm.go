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
	"fmt"
	"os"
	"path"
	"sync"
)

// EphemeralPropertyManager retrieves values for externally controlled properties.
type EphemeralPropertyManager struct {
	Enabled   bool
	loads     int
	arena     string
	arenaPath string // includes global path
	mux       sync.Mutex
	cache     map[string]*Property
}

// NewEPM creates a new EphemeralPropertyManager in a disabled state.
// The specified arena value is pre-pended with the process global arena path if defined.
// The function creates the arena directory if necessary when the package is enabled but does not fail if it cannot do so.
func NewEPM(arena string) *EphemeralPropertyManager {
	epm := &EphemeralPropertyManager{}
	epm.arena = arena
	epm.arenaPath = path.Join(globalArena, arena)
	epm.cache = make(map[string]*Property)
	if Enabled {
		os.Mkdir(epm.arenaPath, ArenaDirPerm)
	}
	return epm
}

// SetProperty is exposed for use by unit tests to prime the cache.
// It blindly replaces the value in the cache, if any.
func (epm *EphemeralPropertyManager) SetProperty(name string, p *Property) {
	epm.mux.Lock()
	defer epm.mux.Unlock()
	epm.cache[name] = p
}

// findProperty locates a property in the cache. Not thread safe.
func (epm *EphemeralPropertyManager) findProperty(name string) *Property {
	var p *Property
	p, present := epm.cache[name]
	if !present && !(Enabled && epm.Enabled) {
		return nil
	}
	if p == nil {
		epm.loads++
		p = NewProperty()
		if p.load(path.Join(epm.arenaPath, name)) != nil {
			return nil
		}
	}
	if !present {
		epm.cache[name] = p
	}
	p.NumUses--
	if p.NumUses <= 0 {
		delete(epm.cache, name)
	}
	return p
}

// GetBool return a boolean property value
func (epm *EphemeralPropertyManager) GetBool(name string) bool {
	epm.mux.Lock()
	defer epm.mux.Unlock()
	if p := epm.findProperty(name); p != nil {
		return p.GetBool()
	}
	return false
}

// GetInt return the typed property value
func (epm *EphemeralPropertyManager) GetInt(name string) int {
	epm.mux.Lock()
	defer epm.mux.Unlock()
	if p := epm.findProperty(name); p != nil {
		return p.GetInt()
	}
	return 0
}

// GetString return the typed property value
func (epm *EphemeralPropertyManager) GetString(name string) string {
	epm.mux.Lock()
	defer epm.mux.Unlock()
	if p := epm.findProperty(name); p != nil {
		return p.GetString()
	}
	return ""
}

// GetFloat return the typed property value
func (epm *EphemeralPropertyManager) GetFloat(name string) float64 {
	epm.mux.Lock()
	defer epm.mux.Unlock()
	if p := epm.findProperty(name); p != nil {
		return p.GetFloat()
	}
	return 0.0
}

// ErrOnBool returns an error if a boolean property value is set
func (epm *EphemeralPropertyManager) ErrOnBool(name string) error {
	if epm.GetBool(name) {
		return fmt.Errorf("injecting error %s/%s", epm.arena, name)
	}
	return nil
}
