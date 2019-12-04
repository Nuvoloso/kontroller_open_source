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


package testutils

import (
	"bytes"
	"encoding/gob"
)

// Clone clones an object using the GOB encoder.
//  Usage:
//   var src Type{initialized}
//   var dst Type
//   GobClone(src, &dst)
func Clone(src interface{}, dstPtr interface{}) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)
	err := enc.Encode(src)
	if err != nil {
		panic("Clone encode: " + err.Error())
	}
	if dstPtr == nil {
		panic("Clone decode: nil destination")
	}
	err = dec.Decode(dstPtr)
	if err != nil {
		panic("Clone decode: " + err.Error())
	}
}
