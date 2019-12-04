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


package metadata

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

var key = []byte{
	0, 1, 2, 3, 4, 5, 6, 7,
	0, 1, 2, 3, 4, 5, 6, 7,
	0, 1, 2, 3, 4, 5, 6, 7,
	0, 1, 2, 3, 4, 5, 6, 7,
}

var badKey = []byte{
	0, 1, 2, 3, 4, 5, 6, 7,
}

func TestMetadataBasic(t *testing.T) {
	var o OffsetT

	if o.String() != "0" {
		t.Error("The Offset string returned unexpected result")
	}

	o = ObjSize - 1
	entry := NewEntry(o, "OBJECT")
	if entry.L1 != 0 || entry.L2 != 0 ||
		entry.L3 != 0 || entry.L4 != 0 {
		fmt.Printf("%v\n", entry)
		t.Error("L0 Entry not constructed properly for 0x" + o.String())
	}
	o = ObjSize
	entry = NewEntry(o, "OBJECT")
	if entry.L1 != 1 || entry.L2 != 0 ||
		entry.L3 != 0 || entry.L4 != 0 {
		fmt.Printf("%v\n", entry)
		t.Error("L1 Entry not constructed properly for 0x" + o.String())
	}
	o <<= OffsetT(lvlSizeBits)
	entry = NewEntry(o, "OBJECT")
	if entry.L1 != 0 || entry.L2 != 1 ||
		entry.L3 != 0 || entry.L4 != 0 {
		fmt.Printf("%v\n", entry)
		t.Error("L2 Entry not constructed properly for 0x" + o.String())
	}
	o <<= OffsetT(lvlSizeBits)
	entry = NewEntry(o, "OBJECT")
	if entry.L1 != 0 || entry.L2 != 0 ||
		entry.L3 != 1 || entry.L4 != 0 {
		fmt.Printf("%v\n", entry)
		t.Error("L3 Entry not constructed properly for 0x" + o.String())
	}
	o <<= OffsetT(lvlSizeBits)
	entry = NewEntry(o, "OBJECT")
	if entry.L1 != 0 || entry.L2 != 0 ||
		entry.L3 != 0 || entry.L4 != 1 {
		fmt.Printf("%v\n", entry)
		t.Error("L4 Entry not constructed properly for 0x" + o.String())
	}

	// Snap Metadata location testing
	md1 := SnapMetadataName(&key, "Volname", "SnapName")
	md2 := SnapMetadataName(nil, "Volname", "SnapName")

	if md1 == md2 {
		t.Error("No key should produce a different metadata name")
	}

	buf := make([]byte, ObjSize)
	for i := 0; i < ObjSize; i++ {
		buf[i] = byte(i % 256)
	}

	hash := DataToHash(buf)
	_ = HashToObjID(hash)

	buffer := bytes.NewBufferString(Header() + "\n")

	var lunSize OffsetT
	lunSize = ObjSize * 20

	for i := OffsetT(0); i < lunSize; i += ObjSize {
		entry := NewEntry(i, EncryptObjName(hash, &key))
		buffer.WriteString(entry.Encode(&key) + "\n")
	}

	mdFileName := "/tmp/mdf"

	mdf, e := os.Create(mdFileName)
	if e != nil {
		t.Error("Can't open temproary file")
	}
	defer os.Remove(mdFileName)

	io.Copy(mdf, buffer)
	mdf.Close()

	r, e := NewMetaReader("bogus")
	if e == nil {
		t.Error("NewMetaReader on bogus file should have failed")
	}

	size, e := SnapSize("bogus", nil)
	if e == nil {
		t.Error("SnapSize(<bogus>, nil) should have failed")
	}

	r, e = NewMetaReader(mdFileName)

	mde := NextEntry(r, &key)
	for mde.Offset() != Infinity {
		mde = NextEntry(r, &key)
	}

	mde = NextEntry(nil, &key)
	if mde.Offset() != Infinity {
		t.Error("NextEntry on a null file should return Infinity")
	}

	size, e = SnapSize(mdFileName, nil)
	if e != nil {
		t.Error("SnapSize Failed" + e.Error())
	}
	if size != lunSize {
		t.Error("size mismatch")
	}
}

func TestMetadataFormat(t *testing.T) {
	buf := make([]byte, ObjSize)
	for i := 0; i < ObjSize; i++ {
		buf[i] = byte(i % 256)
	}

	hash := DataToHash(buf)
	fields := []string{"4", "3", "2", "1", hash}

	entry := fieldsToMetadataEntry(fields, nil)
	if entry.ObjID != hash {
		t.Error("Hash not extracted")
	}
	if entry.L4 != 4 {
		t.Error("L4 not extracted")
	}
	if entry.L3 != 3 {
		t.Error("L3 not extracted")
	}
	if entry.L2 != 2 {
		t.Error("L2 not extracted")
	}
	if entry.L1 != 1 {
		t.Error("L1 not extracted")
	}

	fields = []string{"4", "3", "2", "1", hash}
	fields[3] = "bogus"
	entry = fieldsToMetadataEntry(fields, nil)
	if entry != nil {
		t.Error("Field 3 Bad format undected")
	}

	fields = []string{"4", "3", "2", "1", hash}
	fields[2] = "bogus"
	entry = fieldsToMetadataEntry(fields, nil)
	if entry != nil {
		t.Error("Field 3 Bad format undected")
	}

	fields = []string{"4", "3", "2", "1", hash}
	fields[1] = "bogus"
	entry = fieldsToMetadataEntry(fields, nil)
	if entry != nil {
		t.Error("Field 3 Bad format undected")
	}

	fields = []string{"4", "3", "2", "1", hash}
	fields[0] = "bogus"
	entry = fieldsToMetadataEntry(fields, nil)
	if entry != nil {
		t.Error("Field 3 Bad format undected")
	}

	buffer := bytes.NewBufferString("Bogus,L3,L2,L1,abdced\n")

	mdFileName := "/tmp/mdf"

	mdf, e := os.Create(mdFileName)
	if e != nil {
		t.Error("Can't open temproary file")
	}
	defer os.Remove(mdFileName)

	io.Copy(mdf, buffer)
	mdf.Close()

	_, e = NewMetaReader(mdFileName)
	if e == nil {
		t.Error("NewMetaReader on bogus header should have failed")
	}
}

func TestMetadataDecrypt(t *testing.T) {
	buf := make([]byte, ObjSize)
	for i := 0; i < ObjSize; i++ {
		buf[i] = byte(i % 256)
	}

	hash := DataToHash(buf)
	_ = HashToObjID(hash)

	objName := EncryptObjName(hash, &badKey)

	if objName != "" {
		t.Error("bad key expects empty string")
	}

	_ = EncryptObjName(hash, nil)
	encryptedHash := EncryptObjName(hash, nil)
	if encryptedHash != hash {
		t.Error("nil key should not modify hash")
	}

	clearText := DecryptObjName(hash, nil)
	if encryptedHash != hash {
		t.Error("nil key should not modify hash")
	}

	encryptedHash = EncryptObjName(hash, &key)
	clearText = DecryptObjName(encryptedHash, &key)
	if clearText != hash {
		t.Error("DecryptObjName(" + encryptedHash + ") returns " + clearText + " != " + hash)
	}

	bad := DecryptObjName("12345abcdedf123456", &key)
	if bad != "" {
		t.Error("DecryptObjName should have failed")
	}

	bad = DecryptObjName("bogus", &key)
	if bad != "" {
		t.Error("DecryptObjName should have failed")
	}
}

func TestNameGeneration(t *testing.T) {

	domain := "Domain"
	name := "Name"

	objName := NameToObjID(nil, "", "")
	if objName != "" {
		t.Error("Should be empty name is " + objName)
	}
	objName = NameToObjID(nil, "", name)
	if objName != name {
		t.Error("Should be name is " + objName)
	}
	objName = NameToObjID(nil, domain, "")
	if objName != domain {
		t.Error("Should be domain is " + objName)
	}
	objName = NameToObjID(nil, domain, name)
	if objName != domain+"/"+name {
		t.Error("Should be domain/name is " + objName)
	}
	objName = NameToObjID(&key, "", "")
	// This is undefined but comes out with something

	nameMunged := NameToObjID(&key, "", name)
	if nameMunged == name {
		t.Error("Should not be name is " + nameMunged)
	}
	domainMunged := NameToObjID(&key, domain, "")
	if domainMunged == domain {
		t.Error("Should not be domain is " + domainMunged)
	}
	objName = NameToObjID(&key, domain, name)
	if objName != domainMunged+"/"+nameMunged {
		t.Error("Should be " + domainMunged + "/" + nameMunged)
	}
	part := strings.Split(objName, "/")

	if part[0] != domainMunged {
		t.Error("domain with key should prefix name")
	}
	if part[1] != nameMunged {
		t.Error("name outside domain should match one in domain")
	}
}
