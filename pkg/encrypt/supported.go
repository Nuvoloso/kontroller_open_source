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


package encrypt

import (
	"github.com/Nuvoloso/kontroller/pkg/common"
)

// EncryptionAlgorithm describes a supported encryption algorithm.
type EncryptionAlgorithm struct {
	Name                string
	Description         string
	MinPassphraseLength int32
}

// TBD: methods to make an encryption/decryption session given the pass phrase and protection domain id
// Encryption session object should be reusable
// 2 PDs with same pass phrase should result in different keys

var supportedEncryptionAlgorithms = []EncryptionAlgorithm{
	{
		Name:                common.EncryptionAES256,
		Description:         "AES Encryption with a 256 bit key",
		MinPassphraseLength: 16,
	},
}

// SupportedEncryptionAlgorithms returns the list of supported encryption algorithms
func SupportedEncryptionAlgorithms() []EncryptionAlgorithm {
	return supportedEncryptionAlgorithms
}
