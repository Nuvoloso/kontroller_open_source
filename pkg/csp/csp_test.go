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


package csp

import (
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestCSPFactory(t *testing.T) {
	assert := assert.New(t)

	savedCSPRegistry := cspRegistry
	defer func() {
		cspRegistry = savedCSPRegistry
	}()
	cspRegistry = make(map[models.CspDomainTypeMutable]CloudServiceProvider)

	csp := &fakeCSP{}
	RegisterCSP(csp)
	assert.Len(cspRegistry, 1)
	assert.Contains(cspRegistry, csp.Type())

	c, err := NewCloudServiceProvider("")
	assert.Nil(c)
	assert.Regexp("unsupported", err)

	dts := SupportedCspDomainTypes()
	assert.NotNil(dts)
	assert.Equal(len(dts), len(cspRegistry))
	for _, dt := range dts {
		cSP, err := NewCloudServiceProvider(dt)
		assert.Nil(err)
		assert.NotNil(cSP)
		assert.Equal(dt, cSP.Type())
	}
}

func TestVolumeEnumStrings(t *testing.T) {
	assert := assert.New(t)

	var vps VolumeProvisioningState
	var s string
	for vps, s = range cspVPSMap {
		assert.Equal(s, vps.String())
	}
	s, _ = cspVPSMap[VolumeProvisioningError]
	vps = -1
	assert.Equal(s, vps.String())

	var vas VolumeAttachmentState
	for vas, s = range cspVASMap {
		assert.Equal(s, vas.String())
	}
	s, _ = cspVASMap[VolumeAttachmentError]
	vas = -1
	assert.Equal(s, vas.String())
}

func TestAttributeDescriptionMap(t *testing.T) {
	assert := assert.New(t)

	adm := AttributeDescriptionMap{
		"oD": models.AttributeDescriptor{Kind: common.ValueTypeDuration, Optional: true},
		"oE": models.AttributeDescriptor{Kind: common.ValueTypeSecret, Optional: true},
		"oI": models.AttributeDescriptor{Kind: common.ValueTypeInt, Optional: true},
		"oP": models.AttributeDescriptor{Kind: common.ValueTypePercentage, Optional: true},
		"oS": models.AttributeDescriptor{Kind: common.ValueTypeString, Optional: true},
		"rD": models.AttributeDescriptor{Kind: common.ValueTypeDuration},
		"rE": models.AttributeDescriptor{Kind: common.ValueTypeSecret},
		"rI": models.AttributeDescriptor{Kind: common.ValueTypeInt},
		"rP": models.AttributeDescriptor{Kind: common.ValueTypePercentage},
		"rS": models.AttributeDescriptor{Kind: common.ValueTypeString},
	}

	t.Log("required and optional")
	expNames := []string{}
	attrs := map[string]models.ValueType{}
	for k, ad := range adm {
		attrs[k] = models.ValueType{Kind: ad.Kind}
		expNames = append(expNames, k)
	}
	err := adm.ValidateExpected(expNames, attrs, false, false)
	assert.NoError(err)

	expNames = []string{}
	attrs = map[string]models.ValueType{}
	for k, ad := range adm {
		expNames = append(expNames, k)
		if ad.Optional {
			continue
		}
		attrs[k] = models.ValueType{Kind: ad.Kind}
	}
	assert.True(len(attrs) == len(adm)/2)
	t.Log("exp all, supply required only")
	err = adm.ValidateExpected(expNames, attrs, false, false)
	assert.NoError(err)
	assert.True(len(attrs) == len(adm)/2)
	t.Log("exp all, supply required only, default optional")
	err = adm.ValidateExpected(expNames, attrs, true, false)
	assert.NoError(err)
	assert.True(len(attrs) == len(adm))

	expPattern := "Required: rD\\[D], rE\\[E], rI\\[I], rP\\[%], rS\\[S] Optional: oD\\[D], oE\\[E], oI\\[I], oP\\[%], oS\\[S]"

	t.Log("extra attribute ignored")
	attrs["foo"] = models.ValueType{}
	err = adm.ValidateExpected(expNames, attrs, true, true)
	assert.NoError(err)

	t.Log("extra attribute fails")
	attrs["foo"] = models.ValueType{}
	err = adm.ValidateExpected(expNames, attrs, true, false)
	assert.Error(err)
	assert.Regexp("extra attributes", err)
	assert.Regexp(expPattern, err)

	t.Log("invalid kind")
	oDvT := attrs["oD"]
	attrs["oD"] = models.ValueType{Kind: common.ValueTypeInt}
	err = adm.ValidateExpected(expNames, attrs, true, false)
	assert.Error(err)
	assert.Regexp("invalid Kind for.*oD", err)
	assert.Regexp(expPattern, err)
	attrs["oD"] = oDvT

	t.Log("missing required")
	delete(attrs, "rD")
	err = adm.ValidateExpected(expNames, attrs, true, false)
	assert.Error(err)
	assert.Regexp("missing required attribute.*rD", err)
	assert.Regexp(expPattern, err)

	expNames = []string{"foobar"}
	assert.Panics(func() { err = adm.ValidateExpected(expNames, attrs, true, false) })
}

type fakeCSP struct {
}

var _ = (CloudServiceProvider)(&fakeCSP{})

func (c *fakeCSP) Type() models.CspDomainTypeMutable {
	return "fakeCSP"
}

func (c *fakeCSP) SetDebugLogger(log *logging.Logger) {}

func (c *fakeCSP) Attributes() map[string]models.AttributeDescriptor {
	return nil
}

func (c *fakeCSP) CredentialAttributes() map[string]models.AttributeDescriptor {
	return nil
}

func (c *fakeCSP) DomainAttributes() map[string]models.AttributeDescriptor { return nil }

func (c *fakeCSP) SupportedCspStorageTypes() []*models.CSPStorageType { return nil }

func (c *fakeCSP) InDomain(cspDomainAttrs map[string]models.ValueType, imd map[string]string) error {
	return nil
}

func (c *fakeCSP) Client(cspDomain *models.CSPDomain) (DomainClient, error) { return nil, nil }

func (c *fakeCSP) ValidateCredential(domType models.CspDomainTypeMutable, attrs map[string]models.ValueType) error {
	return nil
}

func (c *fakeCSP) SupportedStorageFormulas() []*models.StorageFormula { return nil }

func (c *fakeCSP) SanitizedAttributes(cspDomainAttrs map[string]models.ValueType) (map[string]models.ValueType, error) {
	return nil, nil
}

func (c *fakeCSP) ProtectionStoreUploadTransferRate() int32 { return 0 }

func (c *fakeCSP) GetDeviceTypeByCspStorageType(storageType models.CspStorageType) (string, error) {
	return "", nil
}

func (c *fakeCSP) LocalInstanceMetadata() (map[string]string, error) {
	return nil, nil
}

func (c *fakeCSP) LocalInstanceMetadataSetTimeout(timeSec int) {}

func (c *fakeCSP) LocalInstanceDeviceName(VolumeIdentifier, AttachedNodeDevice string) (string, error) {
	return "", nil
}
