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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestLocalEphemeralDevices(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)
	cAzure.SetDebugLogger(tl.Logger())

	azureCl := &Client{
		csp: cAzure,
	}
	list, err := azureCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Nil(err)
}
