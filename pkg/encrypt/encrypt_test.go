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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"testing"
)

func TestEncryptBasic(t *testing.T) {
	text := []byte("This is just some random text")
	key := []byte("the-key-has-to-be-32-bytes-long!")

	ciphertext, err := Encrypt(text, key)
	if err != nil {
		t.Error("Encrypt returned error: " + err.Error())
	}

	plaintext, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Error("Decrypt returned error: " + err.Error())
	}

	if bytes.Equal(plaintext, text) == false {
		t.Error("encrypt => decrypt Failed")
	}
}

func TestEncryptBadKeys(t *testing.T) {
	text := []byte("This is just some random text")
	key := []byte("ThisKeyIsTooShort")

	_, err := Encrypt(text, key)
	if err == nil {
		t.Error("Encrypt Should have failed with a short key")
	}

	_, err = Decrypt(text, key)
	if err == nil {
		t.Error("Decrypt Should have failed with a short key")
	}
}

func TestDecryptBadCipher(t *testing.T) {
	text := []byte("T")
	key := []byte("the-key-has-to-be-32-bytes-long!")

	_, err := Decrypt(text, key)
	if err == nil {
		t.Error("Decrypt Should have failed with a short ciphertext")
	}
}

func TestEncryptShort(t *testing.T) {
	text := []byte("This is just some random text")
	key := []byte("the-key-has-to-be-32-bytes-long!")

	for i := len(text); i > 0; i-- {
		ciphertext, err := Encrypt(text[:i], key)
		if err != nil {
			t.Error(fmt.Sprintf("Encrypt iteration: %d returned error: %s", i, err.Error()))
		}

		plaintext, err := Decrypt(ciphertext, key)
		if err != nil {
			t.Error(fmt.Sprintf("Decrypt iteration: %d returned error: %s", i, err.Error()))
		}

		if bytes.Equal(plaintext, text[:i]) == false {
			t.Error("encrypt => decrypt Mismatch")
		}
	}
}

func TestEncryptSymetry(t *testing.T) {
	text := []byte("This is just some random text")
	key := []byte("the-key-has-to-be-32-bytes-long!")

	ciphertext1, err := Encrypt(text, key)
	if err != nil {
		t.Error(fmt.Sprintf("Encrypt returned error: %s", err.Error()))
	}

	ciphertext2, err := Encrypt(text, key)
	if err != nil {
		t.Error(fmt.Sprintf("Encrypt returned error: %s", err.Error()))
	}

	if bytes.Equal(ciphertext1, ciphertext2) == false {
		t.Error("encrypt != encrypt Mismatch")
	}
}

func TestKeyGen(t *testing.T) {
	keySize := 10

	keyNoSalt := PwToKey("PassPhrase", "", keySize)
	keySalt := PwToKey("PassPhrase", "salt", keySize)

	equal := true
	for i := 0; i < keySize; i++ {
		if keyNoSalt[i] != keySalt[i] {
			equal = false
			break
		}
	}
	if equal {
		t.Error("Key with and without salt are equal")
	}
}

func TestNonceSize(t *testing.T) {
	var key [16]byte

	c, err := aes.NewCipher(key[:])
	if err != nil {
		t.Error("NewCipher failed")
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		t.Error("NewGCM failed")
	}

	if len(nonce) < gcm.NonceSize() {
		t.Error("Nonce too small")
	}
}
