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
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	nonceString = "48656c6c6f20476f70686588"
)

var nonce []byte

func init() {
	nonce, _ = hex.DecodeString(nonceString)
}

// PwToKey will take a password/pass phrase and construct a
// key of the specified size. A unique key is generated
// for each pw, salt and size tuple.  Changing any one of these
// 3 will generate a different key.
func PwToKey(pw string, salt string, keySize int) []byte {
	var key []byte

	for keyBytes := 0; keyBytes < keySize; keyBytes = len(key) {
		// We want a different key if the key size is different so the
		// key size is put in the salt. We also resalt on each loop so
		// the key doesn't look repeatable.
		pw = pw + fmt.Sprintf("%x%s", keySize, salt)

		sum := sha256.Sum256([]byte(pw))
		key = append(key, sum[:]...)
	}

	return key[:keySize]
}

// Encrypt returns the ciphertext for the given text and key
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()

	return gcm.Seal(nonce[:nonceSize], nonce[:nonceSize], plaintext, nil), nil
}

// Decrypt returns the plaintext from the ciphertext and key
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	ciphertext = ciphertext[nonceSize:]
	return gcm.Open(nil, nonce[:nonceSize], ciphertext, nil)
}
