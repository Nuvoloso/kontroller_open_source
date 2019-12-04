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
	"fmt"
	"strings"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestGetProtectionDomainMetadata(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	eList := encrypt.SupportedEncryptionAlgorithms()
	pdm := app.GetProtectionDomainMetadata()
	assert.NotNil(pdm)
	assert.NotEmpty(pdm)
	assert.Len(pdm, len(eList)+1)
	last := len(eList)
	for i := 0; i < last; i++ {
		el := eList[i]
		assert.Equal(el.Name, pdm[i].EncryptionAlgorithm)
		assert.Equal(el.Description, pdm[i].Description)
		assert.Equal(el.MinPassphraseLength, pdm[i].MinPassphraseLength)
	}
	assert.Equal(common.EncryptionNone, pdm[last].EncryptionAlgorithm)
	assert.Equal(int32(0), pdm[last].MinPassphraseLength)
}

func TestValidateProtectionDomainEncryptionProperties(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds, Log: tl.Logger()})

	// valid encryption
	eList := encrypt.SupportedEncryptionAlgorithms()
	ep := &models.ValueType{
		Kind:  common.ValueTypeSecret,
		Value: strings.Repeat("x", int(eList[0].MinPassphraseLength)),
	}
	err := app.ValidateProtectionDomainEncryptionProperties(eList[0].Name, ep)
	assert.NoError(err)

	// leading and trailing spaces stripped
	ep.Value = "  \t" + strings.Repeat("x", int(eList[0].MinPassphraseLength)) + " \t "
	err = app.ValidateProtectionDomainEncryptionProperties(eList[0].Name, ep)
	assert.NoError(err)
	assert.Equal(strings.Repeat("x", int(eList[0].MinPassphraseLength)), ep.Value)

	// invalid kind
	ep.Kind = common.ValueTypeString
	err = app.ValidateProtectionDomainEncryptionProperties(eList[0].Name, ep)
	assert.Error(err)
	assert.Regexp("invalid.*Kind", err)

	// too short
	expPat := fmt.Sprintf("encryptionPassphrase.Value less than %d characters", eList[0].MinPassphraseLength)
	ep.Kind = common.ValueTypeSecret
	ep.Value = strings.Repeat("x", int(eList[0].MinPassphraseLength-1))
	err = app.ValidateProtectionDomainEncryptionProperties(eList[0].Name, ep)
	assert.Error(err)
	assert.Regexp(expPat, err)

	// too short after whitespace stripped
	ep.Kind = common.ValueTypeSecret
	ep.Value = strings.Repeat(" ", int(eList[0].MinPassphraseLength))
	err = app.ValidateProtectionDomainEncryptionProperties(eList[0].Name, ep)
	assert.Error(err)
	assert.Regexp(expPat, err)

	// not found
	err = app.ValidateProtectionDomainEncryptionProperties(eList[0].Name+"foo", ep)
	assert.Error(err)
	assert.Regexp("invalid encryptionAlgorithm", err)

	// no-encryption (zaps pass phrase)
	ep.Value = "xxx"
	err = app.ValidateProtectionDomainEncryptionProperties("NONE", ep)
	assert.NoError(err)
	assert.Empty(ep.Value)
}
