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
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
	"io"
	"os"
	"strconv"
)

// OffsetT is the type of a Volume offset
type OffsetT uint64

func (o OffsetT) String() string {
	return fmt.Sprintf("%x", uint64(o))
}

const nuvoBlockSize = 4096
const nuvoMapMask = 0xff
const nuvoMapEntries = nuvoMapMask + 1

// ObjSize is the size of an object
const ObjSize = nuvoMapEntries * nuvoBlockSize
const objSizeBits = 20    // # bits in ObjSize
const lvlSizeBits int = 8 // # bits in nuvoMapEntries

// Infinity is the maximum OffsetT possible
const Infinity = 0xFFFFFFFFFFFFFFFF

// ZeroHash is the hash of a block of zeros
var ZeroHash string

// Entry is an internal representation for an entry in the metadata representing one offset range
type Entry struct {
	L4     uint32
	L3     uint32
	L2     uint32
	L1     uint32
	ObjID  string
	offset OffsetT // Cache of calculated offset
}

// ChunkInfo is the description of a portion of the Volume being backed up
type ChunkInfo struct {
	Offset OffsetT
	Length OffsetT
	Dirty  bool
	Hash   string
	Last   bool
}

func init() {
	buf := make([]byte, ObjSize) // zero initialized
	ZeroHash = DataToHash(buf)
}

// Header generates the Header for a metadata file
func Header() string {
	return fmt.Sprintf("L4,L3,L2,L1,OBJID")
}

// Offset returns the offset of an Entry
func (m *Entry) Offset() OffsetT {
	return m.offset
}

// EncryptObjName Encrypt the Object ID
func EncryptObjName(objID string, key *[]byte) string {
	if key == nil {
		return objID
	}
	arr, err := encrypt.Encrypt([]byte(objID), *key)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%x", arr)
}

// DecryptObjName Decrypt the encrypted name into an Object ID
func DecryptObjName(obj string, key *[]byte) string {
	if key == nil {
		return obj
	}

	var buf []byte

	_, e := fmt.Sscanf(obj, "%x", &buf)
	if e != nil {
		return ""
	}

	arr, err := encrypt.Decrypt(buf, *key)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s", arr)
}

// Encode creates a metadata file line for this entry
func (m *Entry) Encode(key *[]byte) string {
	return fmt.Sprintf("%x,%x,%x,%x,%s", m.L4, m.L3, m.L2, m.L1, EncryptObjName(m.ObjID, key))
}

// fieldsToMetadata allocates a new metadata entry from a set of fields
func fieldsToMetadataEntry(field []string, key *[]byte) *Entry {
	var e error
	var l uint64

	l, e = strconv.ParseUint(field[0], 16, lvlSizeBits)
	if e != nil {
		return nil
	}
	l4 := uint32(l)
	l, e = strconv.ParseUint(field[1], 16, lvlSizeBits)
	if e != nil {
		return nil
	}
	l3 := uint32(l)
	l, e = strconv.ParseUint(field[2], 16, lvlSizeBits)
	if e != nil {
		return nil
	}
	l2 := uint32(l)
	l, e = strconv.ParseUint(field[3], 16, lvlSizeBits)
	if e != nil {
		return nil
	}
	l1 := uint32(l)

	off := OffsetT((l4 & nuvoMapMask))
	off = (off << OffsetT(lvlSizeBits)) | (OffsetT(l3) & nuvoMapMask)
	off = (off << OffsetT(lvlSizeBits)) | (OffsetT(l2) & nuvoMapMask)
	off = (off << OffsetT(lvlSizeBits)) | (OffsetT(l1) & nuvoMapMask)
	off <<= objSizeBits

	objID := DecryptObjName(field[4], key)

	return &Entry{
		L4:     l4,
		L3:     l3,
		L2:     l2,
		L1:     l1,
		ObjID:  objID,
		offset: off,
	}
}

// NewEntry allocates a new metadata entry from offset and object ID
func NewEntry(offset OffsetT, objname string) *Entry {
	off := uint32(offset >> objSizeBits)
	L1 := off & nuvoMapMask
	off >>= OffsetT(lvlSizeBits)
	L2 := off & nuvoMapMask
	off >>= OffsetT(lvlSizeBits)
	L3 := off & nuvoMapMask
	off >>= OffsetT(lvlSizeBits)
	L4 := off & nuvoMapMask

	return &Entry{L4, L3, L2, L1, objname, offset}
}

// SnapMetadataName returns the name of the per snapshot metadata
func SnapMetadataName(key *[]byte, domain string, snapshot string) string {
	return NameToObjID(key, domain, snapshot)
}

// HashToObjID returns and object name from a data hash
func HashToObjID(hash string) string {
	return "data/" + hash
}

// NameToObjID puts together the name for the object
func NameToObjID(key *[]byte, domain string, name string) string {
	if key == nil {
		if domain != "" && name != "" {
			return domain + "/" + name
		}
		// one is "" so just return one of them
		return domain + name
	}

	var prefix string
	if domain != "" {
		prefix = EncryptObjName(domain, key)
		if name == "" {
			return prefix
		}
		prefix = prefix + "/"
	}
	return prefix + EncryptObjName(name, key)
}

// NewMetaReader creates a reader to use for parsing metadata files
func NewMetaReader(metadatafn string) (*csv.Reader, error) {
	mdf, e := os.Open(metadatafn)
	if e != nil {
		return nil, errors.New("Metadata file header open failed")
	}
	r := csv.NewReader(mdf)

	// Ignore the header
	fields, e := r.Read()
	if e != io.EOF && fields[0] != "L4" {
		return nil, errors.New("Metadata file header problem")
	}
	return r, nil
}

// NextEntry returns an Entry representing the next line in the metadata file
func NextEntry(r *csv.Reader, key *[]byte) *Entry {
	// Should probably return a possible error here
	if r == nil {
		return &Entry{offset: Infinity}
	}
	fields, e := r.Read()
	if e == io.EOF {
		return &Entry{offset: Infinity}
	}
	return fieldsToMetadataEntry(fields, key)
}

// DataToHash returns the hash string for the data given
func DataToHash(buf []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(buf))
}

// SnapSize returns the size of the LUN represented by the snapshot metadata file
func SnapSize(fn string, key *[]byte) (OffsetT, error) {
	var offset OffsetT

	reader, err := NewMetaReader(fn)
	if err != nil {
		return 0, err
	}

	mde := NextEntry(reader, key)
	for mde.Offset() != Infinity {
		offset = mde.Offset()
		mde = NextEntry(reader, key)
	}
	return offset + OffsetT(ObjSize), nil
}
