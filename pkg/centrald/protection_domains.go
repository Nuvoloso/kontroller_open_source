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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
)

// GetProtectionDomainMetadata describes the supported encryption mechanisms
func (app *AppCtx) GetProtectionDomainMetadata() []*models.ProtectionDomainMetadata {
	eList := encrypt.SupportedEncryptionAlgorithms()
	list := make([]*models.ProtectionDomainMetadata, 0, len(eList)+1)
	for _, ea := range eList {
		list = append(list, &models.ProtectionDomainMetadata{
			EncryptionAlgorithm: ea.Name,
			Description:         ea.Description,
			MinPassphraseLength: ea.MinPassphraseLength,
		})
	}
	list = append(list, &models.ProtectionDomainMetadata{
		EncryptionAlgorithm: common.EncryptionNone,
		Description:         "No encryption is performed",
	})
	return list
}

// ValidateProtectionDomainEncryptionProperties checks creation time encryption parameters
// The encryptionPassphrase value will be empty if the encryptionAlgorithm specified is NONE.
// Leading and trailing whitespace is stripped from the passphrase.
func (app *AppCtx) ValidateProtectionDomainEncryptionProperties(encryptionAlgorithm string, encryptionPassphrase *models.ValueType) error {
	for _, pdm := range app.GetProtectionDomainMetadata() {
		encryptionPassphrase.Value = strings.TrimSpace(encryptionPassphrase.Value)
		if pdm.EncryptionAlgorithm == encryptionAlgorithm {
			if len(encryptionPassphrase.Value) < int(pdm.MinPassphraseLength) {
				return fmt.Errorf("encryptionPassphrase.Value less than %d characters", pdm.MinPassphraseLength)
			}
			if encryptionPassphrase.Kind != common.ValueTypeSecret {
				return fmt.Errorf("invalid encryptionPassphrase.Kind")
			}
			if pdm.MinPassphraseLength == 0 {
				encryptionPassphrase.Value = "" // zap
			}
			return nil
		}
	}
	return fmt.Errorf("invalid encryptionAlgorithm")
}
