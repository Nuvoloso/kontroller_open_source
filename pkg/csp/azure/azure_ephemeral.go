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


package azure

import (
	"github.com/Nuvoloso/kontroller/pkg/csp"
)

// LocalEphemeralDevices satisfies the EphemeralDiscoverer interface
func (cl *Client) LocalEphemeralDevices() ([]*csp.EphemeralDevice, error) {
	// Ephemeral devices are not defined in Azure.
	// Need to explore the use of the temporary device for this purpose.
	return nil, nil
}
