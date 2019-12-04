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
)

// Property contains data and meta-data of a property
type Property struct {
	loaded      bool
	BoolValue   bool    `json:"boolValue,omitempty"`
	IntValue    int     `json:"intValue,omitempty"`
	StringValue string  `json:"stringValue,omitempty"`
	FloatValue  float64 `json:"floatValue,omitempty"`
	NumUses     int     `json:"numUses,omitempty"`
	DoNotDelete bool    `json:"doNotDelete,omitempty"`
}

// NewProperty returns a new Property object
func NewProperty() *Property {
	return &Property{}
}

// load a property file then delete the file if necessary
func (p *Property) load(fn string) error {
	b, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}
	p.loaded = true
	if len(b) != 0 {
		json.Unmarshal(b, &p)
	} else {
		p.BoolValue = true // set bool true if file truly empty
	}
	if p.NumUses <= 0 {
		p.NumUses = 1
	}
	if !p.DoNotDelete {
		os.Remove(fn)
	}
	return nil
}

// GetBool returns a boolean value
func (p *Property) GetBool() bool {
	return p.BoolValue
}

// GetInt returns an int value
func (p *Property) GetInt() int {
	return p.IntValue
}

// GetString returns a string value
func (p *Property) GetString() string {
	return p.StringValue
}

// GetFloat returns a float64 value
func (p *Property) GetFloat() float64 {
	return p.FloatValue
}
