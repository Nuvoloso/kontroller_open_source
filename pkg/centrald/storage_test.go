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


package centrald

import (
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestStorageTypes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})
	cspStorageTypes := app.SupportedCspStorageTypes()
	for _, st := range cspStorageTypes {
		assert.True(swag.Int64Value(st.PreferredAllocationSizeBytes) >= swag.Int64Value(st.MinAllocationSizeBytes))
		assert.True(swag.Int64Value(st.PreferredAllocationSizeBytes) <= swag.Int64Value(st.MaxAllocationSizeBytes))
		assert.True(swag.Int64Value(st.MinAllocationSizeBytes) <= swag.Int64Value(st.MaxAllocationSizeBytes))
	}
}

func TestStorageAccessibility(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	spas := app.SupportedStorageAccessibilityScopes()
	assert.NotNil(spas)
	assert.Len(spas, 2)
	cspStorageTypes := app.SupportedCspStorageTypes()
	var nodeST, cspDomainST *models.CSPStorageType
	for _, st := range cspStorageTypes {
		if st.AccessibilityScope == "NODE" && nodeST == nil {
			nodeST = st
		} else if st.AccessibilityScope == "CSPDOMAIN" && cspDomainST == nil {
			cspDomainST = st
		}
	}
	assert.NotNil(nodeST)
	assert.NotNil(cspDomainST)
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", cspDomainST.Name, nil))
	assert.NoError(app.ValidateStorageAccessibility("cspDomainID", nodeST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "NODE", AccessibilityScopeObjID: "id"}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", nodeST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "NODE", AccessibilityScopeObjID: ""}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", nodeST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "NODEFoo", AccessibilityScopeObjID: "id"}))
	assert.NoError(app.ValidateStorageAccessibility("cspDomainID", cspDomainST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "CSPDOMAIN", AccessibilityScopeObjID: "cspDomainID"}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", cspDomainST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "CSPDOMAIN", AccessibilityScopeObjID: ""}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", cspDomainST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "CSPDOMAIN", AccessibilityScopeObjID: "id"}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", cspDomainST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "CSPDOMAINIDFoo", AccessibilityScopeObjID: "cspDomainID"}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", cspDomainST.Name+"XXX", &models.StorageAccessibilityMutable{AccessibilityScope: "CSPDOMAIN", AccessibilityScopeObjID: "cspDomainID"}))
	assert.Error(app.ValidateStorageAccessibility("cspDomainID", nodeST.Name, &models.StorageAccessibilityMutable{AccessibilityScope: "CSPDOMAIN", AccessibilityScopeObjID: "cspDomainID"}))
}

func TestStorageAttachmentState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sps := app.SupportedStorageAttachmentStates()
	assert.NotNil(sps)
	assert.NotEmpty(sps)
	assert.Equal(sps, supportedStorageAttachmentStates)
	assert.False(app.ValidateStorageAttachmentState(""))
	for _, sp := range sps {
		assert.True(sp != "")
		assert.True(app.ValidateStorageAttachmentState(sp))
		assert.False(app.ValidateStorageAttachmentState(sp + "x"))
	}
}

func TestStorageDeviceState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sps := app.SupportedStorageDeviceStates()
	assert.NotNil(sps)
	assert.NotEmpty(sps)
	assert.Equal(sps, supportedStorageDeviceStates)
	assert.False(app.ValidateStorageDeviceState(""))
	for _, sp := range sps {
		assert.True(sp != "")
		assert.True(app.ValidateStorageDeviceState(sp))
		assert.False(app.ValidateStorageDeviceState(sp + "x"))
	}
}

func TestStorageMediaState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sps := app.SupportedStorageMediaStates()
	assert.NotNil(sps)
	assert.NotEmpty(sps)
	assert.Equal(sps, supportedStorageMediaStates)
	assert.False(app.ValidateStorageMediaState(""))
	for _, sp := range sps {
		assert.True(sp != "")
		assert.True(app.ValidateStorageMediaState(sp))
		assert.False(app.ValidateStorageMediaState(sp + "x"))
	}
}

func TestStorageProvisionedState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sps := app.SupportedStorageProvisionedStates()
	assert.NotNil(sps)
	assert.NotEmpty(sps)
	assert.Equal(sps, supportedStorageProvisionedStates)
	assert.False(app.ValidateStorageProvisionedState(""))
	for _, sp := range sps {
		assert.True(sp != "")
		assert.True(app.ValidateStorageProvisionedState(sp))
		assert.False(app.ValidateStorageProvisionedState(sp + "x"))
	}
}

func TestStorageRequestState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	sps := app.SupportedStorageRequestStates()
	assert.NotNil(sps)
	assert.NotEmpty(sps)
	assert.Equal(sps, supportedStorageRequestStates)
	assert.False(app.ValidateStorageRequestState(""))
	for _, sp := range sps {
		assert.True(sp != "")
		assert.True(app.ValidateStorageRequestState(sp))
		assert.False(app.ValidateStorageRequestState(sp + "x"))
	}
}

func TestStorageTerminalStates(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	states := app.TerminalStorageRequestStates()
	assert.Equal([]string{com.StgReqStateSucceeded, com.StgReqStateFailed}, states)
}
