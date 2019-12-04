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


package stats

import (
	"encoding/json"
	"testing"
)

type sampleJSON struct {
	A int64
	B int64
	C int64
}

func TestStatsBasic(t *testing.T) {
	stats := Stats{}

	stats.Init() // Set up
	stats.Inc("A")
	if stats.Get("A") != 1 {
		t.Error("Inc")
	}
	stats.Dec("B")
	if stats.Get("B") != -1 {
		t.Error("Dec")
	}
	stats.Add("B", 2)
	if stats.Get("B") != 1 {
		t.Error("Add")
	}
	stats.Sub("B", 2)
	if stats.Get("B") != -1 {
		t.Error("Sub")
	}
	stats.Set("B", 5)
	if stats.Get("B") != 5 {
		t.Error("Sec")
	}

	b, e := stats.MarshalJSON()
	if e != nil {
		t.Error("MarshalJSON")
	}

	var jsonSample sampleJSON

	json.Unmarshal(b, &jsonSample)

	if jsonSample.A != stats.Get("A") {
		t.Error("json decode A failed")
	}

	if jsonSample.B != stats.Get("B") {
		t.Error("json decode B failed")
	}
}
