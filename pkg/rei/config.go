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

import "os"

// Enabled must be set to true to enable use of this package at runtime.
var Enabled bool

const (
	// ArenaDirPerm is the permission for the arena directories
	ArenaDirPerm os.FileMode = 0700
)

// globalArena is a path that is prepended to the arena names of individual injectors.
// It can be used to establish a per-process location.
var globalArena string

// SetGlobalArena specifies the directory to use for the process.
// The directory will be created if necessary if the package is enabled.
func SetGlobalArena(dir string) error {
	globalArena = dir
	if Enabled {
		return os.MkdirAll(globalArena, ArenaDirPerm)
	}
	return nil
}
