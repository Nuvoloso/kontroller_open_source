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


package endpoint

import (
	"encoding/json"
	"errors"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
)

// Purpose is the reason for the endpoint: source or Destination
type Purpose int

const (
	// Source Endpoint
	source Purpose = iota
	// Destination Endpoint
	destination
	// Manipulation Endpoint
	manipulation
	// Biggest size of object to read
	maxRetreiveSize = 1024 * 1024 * 2

	// StatNameDiffBytesChanged Byte count Differ found changed
	StatNameDiffBytesChanged = "BytesChanged"
	// StatNameDiffBytesUnchanged Byte count Differ found unchanged
	StatNameDiffBytesUnchanged = "BytesUnchanged"
	// StatNameSrcBytesRead Bytes read from the source
	StatNameSrcBytesRead = "BytesReadSrc"
	// StatNameSrcReadCount number of reads from the source
	StatNameSrcReadCount = "SrcReadCount"
	// StatNameBytesPreDedup Bytes might transfer (if no deduped)
	StatNameBytesPreDedup = "PrededupBytes"
	// StatNameDestBytesWritten Bytes sent to destination
	StatNameDestBytesWritten = "BytesTransferred"
	// StatNameDestBytesSkipped Bytes clean from destination side
	StatNameDestBytesSkipped = "DestBytesSkipped"
	// StatNameDestBytesDedup Bytes not transferred because dedup
	StatNameDestBytesDedup = "DestBytesDeduped"
	// StatNameDiffGaps Count of ranges not sent by Differ
	StatNameDiffGaps = "DiffIgnored"
	// StatNameProgress Progress on the LUN backup
	StatNameProgress = "OffsetProgress"
	// StatNameReadZeroSkipped Bytes not read because all zeros
	StatNameReadZeroSkipped = "ZeroBytesNotFetched"
	// StatNameWriteZeroSkipped Zeroed Bytes Not written
	StatNameWriteZeroSkipped = "ZeroBytesNotWritten"

	// TypeGoogle Type String for Google endpoints
	TypeGoogle = "Google"
	// TypeAWS Type String for AWS S3 endpoints
	TypeAWS = "S3"
	// TypeAzure Type String for Azure endpoints
	TypeAzure = "Azure"
	// TypeDir Type String for Dir endpoints
	TypeDir = "Dir"
	// TypeNuvo Type String for Nuvoloso endpoints
	TypeNuvo = "Nuvo"
	// TypeFile Type String for File endpoints
	TypeFile = "File"
	// TypeFake Type String for Fake endpoints
	TypeFake = "Fake"
)

func (p Purpose) String() string {
	switch p {
	case source:
		return "Source"
	case destination:
		return "Destination"
	case manipulation:
		return "Manipulation"
	default:
		return "Unknown"
	}
}

// AbortCheck is the type of function used to determine
// if processing should abort.
type AbortCheck func() bool

// AllArgs is the collection of arguments to all types of endpoints
// used for JSON encoding.
type AllArgs struct {
	Azure  AzArgs
	Dir    DirArgs
	Fake   FakeArgs
	File   FileArgs
	Google GcArgs
	Nuvo   NuvoArgs
	S3     S3Args
}

// CopyArgs encapsulates the description of what copy should do
// Used for JSON encoding/decoding
type CopyArgs struct {
	SrcType          string
	SrcArgs          AllArgs
	DstType          string
	DstArgs          AllArgs
	ManType          string
	ManArgs          AllArgs
	ProgressFileName string
	NumThreads       uint
}

// Arg holds arguments for a single endpoint
type Arg struct {
	Purpose string
	Type    string
	Args    AllArgs
}

// EndPoint is the interface all endpoints should implement
type EndPoint interface {
	Setup() error
	Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error
	FetchAt(offset metadata.OffsetT, hint string, buf []byte) error
	FillAt(offset metadata.OffsetT, hint string, buf []byte) error
	SkipAt(offset metadata.OffsetT, hint string) error
	GetIncrSnapName() string
	GetLunSize() int64
	Done(success bool) error
	// Catalog style access to the protection store
	StoreData(string, []byte) error
	RetreiveData(string, []byte) (int, error)
	GetListNext(ctx interface{}) (string, interface{}, error)
}

// SetupEndpoint does what is needed to make an endpoint functional
func SetupEndpoint(args *Arg, stats *stats.Stats, workerCount uint) (EndPoint, error) {
	var purpose Purpose

	switch args.Purpose {
	case "Source":
		purpose = source
	case "Destination":
		purpose = destination
	case "Manipulation":
		purpose = manipulation
	default:
		return nil, errors.New("Unsupported Endpoint Purpose: " + args.Purpose)
	}

	var ep EndPoint

	switch args.Type {
	case TypeAzure:
		ep = &azEndpoint{args: &args.Args.Azure,
			stats:       stats,
			purpose:     purpose,
			workerCount: workerCount,
		}
	case TypeDir:
		ep = &dirEndpoint{args: &args.Args.Dir,
			stats:       stats,
			purpose:     purpose,
			workerCount: workerCount,
		}
	case TypeFake:
		ep = &fakeEndpoint{args: &args.Args.Fake,
			stats:   stats,
			purpose: purpose,
		}
	case TypeFile:
		ep = &fileEndpoint{args: &args.Args.File,
			stats:   stats,
			purpose: purpose,
		}
	case TypeGoogle:
		ep = &gcEndpoint{args: &args.Args.Google,
			stats:       stats,
			purpose:     purpose,
			workerCount: workerCount,
		}
	case TypeNuvo:
		ep = &nuvoEndpoint{args: &args.Args.Nuvo,
			stats:   stats,
			purpose: purpose,
		}
	case TypeAWS:
		ep = &s3Endpoint{args: &args.Args.S3,
			stats:       stats,
			purpose:     purpose,
			workerCount: workerCount,
		}
	default:
		return nil, errors.New("Unsupported Endpoint Type: " + args.Type)
	}

	err := ep.Setup()
	if err != nil {
		return nil, err
	}
	return ep, nil
}

// PutJSONCatalog utility to put the JSON encoded structure into the catalog.
func PutJSONCatalog(ep EndPoint, name string, thing interface{}) error {
	buf, err := json.Marshal(&thing)
	if err != nil {
		return errors.New("Marshal: " + err.Error())
	}
	err = ep.StoreData(name, buf)
	if err != nil {
		return errors.New("StoreData: " + err.Error())
	}
	return nil
}

// GetJSONCatalog utility to get the JSON encoded structure from the catalog.
func GetJSONCatalog(ep EndPoint, name string, thing interface{}) error {
	var buf [maxRetreiveSize]byte

	n, e := ep.RetreiveData(name, buf[:])
	if e != nil {
		return errors.New("RetreiveData: " + e.Error())
	}

	e = json.Unmarshal(buf[:n], thing)
	if e != nil {
		return errors.New("RetreiveData: " + e.Error())
	}
	return nil
}
