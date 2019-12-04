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


package mongods

import "context"

// Populator is a type that exposes functions required to populate a collection
type Populator interface {
	// CName returns the name of the collection.
	CName() string
	// Populate the collection with a marshaled JSON object, typically a marshaled model object.
	// The function determines if population is required, whether the byte array is correct JSON, etc.
	Populate(ctx context.Context, path string, buf []byte) error
}
