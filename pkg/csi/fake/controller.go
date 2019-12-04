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


package fake

// ControllerHandler fakes its namesake
type ControllerHandler struct {
	CalledStart bool
	RetStartErr error

	CalledStop bool
}

// Start fakes its namesake
func (h *ControllerHandler) Start() error {
	h.CalledStart = true
	return h.RetStartErr
}

// Stop fakes its namesake
func (h *ControllerHandler) Stop() {
	h.CalledStop = true
}
