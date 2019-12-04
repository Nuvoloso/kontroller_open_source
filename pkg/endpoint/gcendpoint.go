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
	"bytes"
	"cloud.google.com/go/storage"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sync"
	"time"
)

// GcArgs is the set of arguments expected for an Google copy endpoint
type GcArgs struct {
	BucketName string
	Domain     string
	Base       string
	Incr       string
	PassPhrase string
	Cred       string
}

type gcEndpoint struct {
	args         *GcArgs
	purpose      Purpose
	workerCount  uint
	bucketName   string
	baseSnapName string
	incrSnapName string
	baseReader   *csv.Reader
	incrReader   *csv.Reader
	mdChan       chan *metadata.Entry
	mdwg         *sync.WaitGroup
	lunSize      int64
	client       *storage.Client
	bucket       *storage.BucketHandle
	stats        *stats.Stats
	key          *[]byte
	id           string
	err          error
}

func (gcep *gcEndpoint) Setup() error {
	var err error
	args := gcep.args

	gcep.bucketName = args.BucketName
	gcep.baseSnapName = args.Base
	gcep.incrSnapName = args.Incr
	rand.Seed(int64(os.Getpid()))
	gcep.id = fmt.Sprintf("%x", rand.Uint64())

	if args.Domain == "" {
		return errors.New("Google protection domain must be set")
	}

	if args.PassPhrase != "" {
		b := encrypt.PwToKey(args.PassPhrase, args.Domain, 32)
		gcep.key = &b
	}

	ctx := context.Background()
	if gcep.client, err = storage.NewClient(ctx, option.WithCredentialsJSON([]byte(args.Cred))); err != nil {
		return errors.New("Google client: " + err.Error())
	}

	// create a bucket handler
	gcep.bucket = gcep.client.Bucket(gcep.bucketName)

	switch gcep.purpose {
	case destination:
		// We need a metadata handler for a destination
		if gcep.baseSnapName != "" {
			baseSnapObjectName := metadata.SnapMetadataName(gcep.key,
				gcep.args.Domain,
				gcep.baseSnapName)
			present, err := gcep.objExists(baseSnapObjectName)
			if err != nil {
				return errors.New("Exists: " + err.Error())
			}
			if !present {
				return errors.New("Base Snapshot Not Present")
			}
			err = gcep.getObjectFile(baseSnapObjectName, "/tmp/base_gc_"+gcep.id)
			if err != nil {
				return errors.New("retrieve base snapshot " + err.Error())
			}

			gcep.baseReader, err = metadata.NewMetaReader("/tmp/base_gc_" + gcep.id)
			if err != nil {
				return errors.New("setup base snapshot reader " + err.Error())
			}
		}
		gcep.mdwg = &sync.WaitGroup{}
		gcep.mdwg.Add(1)
		gcep.mdChan = make(chan *metadata.Entry, gcep.workerCount*2)
		go gcMetadataHandler(gcep)
	case source:
		if gcep.baseSnapName == "" {
			gcep.baseReader, err = metadata.NewMetaReader("/dev/null")
			if err != nil {
				return errors.New("setup null snapshot reader " + err.Error())
			}
		} else {
			baseSnapObjectName := metadata.SnapMetadataName(gcep.key,
				gcep.args.Domain,
				gcep.baseSnapName)
			err = gcep.getObjectFile(baseSnapObjectName, "/tmp/base_gc_"+gcep.id)
			if err != nil {
				return errors.New("retrieve base snapshot " + err.Error())
			}

			gcep.baseReader, err = metadata.NewMetaReader("/tmp/base_gc_" + gcep.id)
			if err != nil {
				return errors.New("setup base snapshot reader " + err.Error())
			}
		}
		incrSnapObjectName := metadata.SnapMetadataName(gcep.key,
			gcep.args.Domain,
			gcep.incrSnapName)
		err = gcep.getObjectFile(incrSnapObjectName, "/tmp/incr_gc_"+gcep.id)
		if err != nil {
			return errors.New("retrieve incr snapshot " + err.Error())
		}

		size, err := metadata.SnapSize("/tmp/incr_gc_"+gcep.id, gcep.key)
		if err != nil {
			return errors.New("getting lun size: " + err.Error())
		}

		gcep.lunSize = int64(size)
		gcep.incrReader, err = metadata.NewMetaReader("/tmp/incr_gc_" + gcep.id)
		if err != nil {
			return errors.New("setup incremental snapshot reader " + err.Error())
		}
		gcep.stats.Set(StatNameSrcReadCount, 0)

	case manipulation:
		// Nothing to do
	default:
		return errors.New("Invalid Purpose for Dir Endpoint")
	}

	if gcep.purpose == source || gcep.purpose == destination {
		var baseName string
		if gcep.baseSnapName == "" {
			baseName = "null"
		} else {
			baseName = gcep.baseSnapName
		}

		fmt.Printf("%s Google Endpoint on %s: %s %s -> %s\n", gcep.purpose.String(), gcep.bucketName,
			gcep.args.Domain, baseName, gcep.incrSnapName)
	}

	return nil
}

// Could move this to metadata package
func (gcep *gcEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)
	maxSleepTime := time.Millisecond * 500

	// should make the meta chan internal
	baseMDE := metadata.NextEntry(gcep.baseReader, gcep.key)
	incrMDE := metadata.NextEntry(gcep.incrReader, gcep.key)

	for incrMDE.Offset() != metadata.Infinity {
		if shouldAbort() {
			return errors.New("Stopped due to abort condition")
		}
		var chunk *metadata.ChunkInfo
		chunk = nil

		if incrMDE.Offset() == baseMDE.Offset() {
			if incrMDE.ObjID == baseMDE.ObjID {
				// No Difference
				gcep.stats.Add(StatNameDiffBytesUnchanged, metadata.ObjSize)
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  false,
				}
			} else {
				// Difference
				gcep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  true,
				}
			}
			baseMDE = metadata.NextEntry(gcep.baseReader, gcep.key)
			incrMDE = metadata.NextEntry(gcep.incrReader, gcep.key)
		} else if incrMDE.Offset() < baseMDE.Offset() {
			// Difference
			gcep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
			chunk = &metadata.ChunkInfo{
				Offset: incrMDE.Offset(),
				Length: metadata.ObjSize,
				Hash:   incrMDE.ObjID,
				Dirty:  true,
			}
			incrMDE = metadata.NextEntry(gcep.incrReader, gcep.key)
		} else if incrMDE.Offset() > baseMDE.Offset() {
			// How can this happen?
			// Since fixed Span Size not a diff
			baseMDE = metadata.NextEntry(gcep.baseReader, gcep.key)
		}

		sleepTime := time.Millisecond

		for chunk != nil {
			select {
			case dataChan <- chunk:
				chunk = nil
			default:
				time.Sleep(sleepTime)
				if shouldAbort() {
					return errors.New("Stopped due to abort condition")
				}
				if sleepTime < maxSleepTime {
					sleepTime = sleepTime * 2
				} else {
					sleepTime = maxSleepTime
				}
			}
		}
	}

	return nil
}

func (gcep *gcEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if hint == "" {
		return errors.New("Hint Not Set")
	}

	if hint == metadata.ZeroHash {
		gcep.stats.Add(StatNameReadZeroSkipped, metadata.ObjSize)
		for i := 0; i < metadata.ObjSize; i++ {
			buf[i] = 0
		}
		return nil
	}

	objName := metadata.NameToObjID(gcep.key, gcep.args.Domain, hint)

	n, e := gcep.getObjectBytes(objName, buf)
	if e != nil {
		return errors.New("Google get object bytes: " + e.Error())
	}

	if n != metadata.ObjSize {
		return errors.New("google object size not correct")
	}

	gcep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)
	gcep.stats.Inc(StatNameSrcReadCount)

	return nil
}

func (gcep *gcEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if hint == "" {
		hint = metadata.DataToHash(buf)
	}

	objName := metadata.NameToObjID(gcep.key, gcep.args.Domain, hint)

	gcep.stats.Add(StatNameBytesPreDedup, metadata.ObjSize)

	present, err := gcep.objExists(objName)
	if err != nil {
		return errors.New("existence check: " + err.Error())
	}
	if present {
		gcep.stats.Add(StatNameDestBytesDedup, metadata.ObjSize)
		gcep.mdChan <- metadata.NewEntry(offset, hint)
		return nil
	}

	err = gcep.putObjectBytes(objName, buf)
	if err != nil {
		return errors.New("put object bytes: " + err.Error())
	}
	gcep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)

	gcep.mdChan <- metadata.NewEntry(offset, hint)

	return nil
}

func (gcep *gcEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	gcep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	gcep.mdChan <- metadata.NewEntry(offset, hint)
	return nil
}

func (gcep *gcEndpoint) GetIncrSnapName() string {
	return gcep.incrSnapName
}

func (gcep *gcEndpoint) GetLunSize() int64 {
	return gcep.lunSize
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (gcep *gcEndpoint) StoreData(name string, data []byte) error {
	objName := metadata.NameToObjID(gcep.key, gcep.args.Domain, name)

	if err := gcep.putObjectBytes(objName, data); err != nil {
		gcep.err = errors.New("put object: " + err.Error())
		return gcep.err
	}

	return nil
}

// ResteiveData is a catalog read function
func (gcep *gcEndpoint) RetreiveData(name string, data []byte) (int, error) {
	var n int

	objName := metadata.NameToObjID(gcep.key, gcep.args.Domain, name)

	n, err := gcep.getObjectBytes(objName, data)
	if err != nil {
		gcep.err = errors.New("get object: " + err.Error())
		return 0, gcep.err
	}

	return n, nil
}

type gcListIteratorCtx struct {
	it *storage.ObjectIterator
}

func (gcep *gcEndpoint) GetListNext(genericCtx interface{}) (string, interface{}, error) {
	var gcctx *gcListIteratorCtx

	if genericCtx == nil {
		gcctx = &gcListIteratorCtx{}
		prefix := metadata.NameToObjID(gcep.key, gcep.args.Domain, "") + "/"

		ctx := context.Background()

		q := &storage.Query{Prefix: prefix}
		gcctx.it = gcep.bucket.Objects(ctx, q)
	} else {
		gcctx = genericCtx.(*gcListIteratorCtx)
	}

	i, e := gcctx.it.Next()
	if e == iterator.Done {
		return "", nil, nil
	}

	return metadata.DecryptObjName(path.Base(i.Name), gcep.key), gcctx, nil
}

func (gcep *gcEndpoint) Done(success bool) error {
	switch gcep.purpose {
	case source:
		_ = os.Remove("/tmp/incr_gc_" + gcep.id)
		_ = os.Remove("/tmp/base_gc_" + gcep.id)
	case destination:
		// Close the mdChan to stop the metadata handler
		close(gcep.mdChan)
		// Wait for it to finish
		gcep.mdwg.Wait()
		_ = os.Remove("/tmp/base_gc_" + gcep.id)
	case manipulation:
		// Nothing to do
	default:
		fmt.Printf("Done on a bad Endpoint.")
	}
	return gcep.err
}

func (gcep *gcEndpoint) getObjectBytes(objName string, buf []byte) (int, error) {
	var err error

	obj := gcep.bucket.Object(objName)
	ctx := context.TODO()
	r, err := obj.NewReader(ctx)
	if err != nil {
		return 0, errors.New("google new reader: " + err.Error())
	}
	defer r.Close()

	dbuf, err := ioutil.ReadAll(r)
	if err != nil {
		return 0, errors.New("google get object bytes readall: " + err.Error())
	}

	var length int

	if gcep.key == nil {
		length = len(dbuf)
		copy(buf, dbuf[:length])
	} else {
		dataBuf, err := encrypt.Decrypt(dbuf, *gcep.key)
		if err != nil {
			return 0, errors.New("google decrypt: " + err.Error())
		}
		copy(buf, dataBuf[:len(dataBuf)])
		length = len(dataBuf)

	}

	return length, nil
}

func (gcep *gcEndpoint) getObjectFile(ObjName string, fname string) error {
	f, err := os.Create(fname)
	if err != nil {
		return errors.New("Create " + err.Error())
	}
	defer f.Close()

	w := io.Writer(f)

	obj := gcep.bucket.Object(ObjName)
	ctx := context.TODO()
	r, err := obj.NewReader(ctx)
	if err != nil {
		return errors.New("NewWriter " + err.Error())
	}
	defer r.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return errors.New("Copy " + err.Error())
	}

	return nil
}

func (gcep *gcEndpoint) putObjectBytes(objName string, buf []byte) error {
	obj := gcep.bucket.Object(objName)
	ctx := context.TODO()
	w := obj.NewWriter(ctx)
	defer w.Close()

	if gcep.key == nil {
		buffer := bytes.NewBuffer(buf)

		_, err := io.Copy(w, buffer)
		if err != nil {
			return errors.New("Copy " + err.Error())
		}
	} else {
		cipherBuf, err := encrypt.Encrypt(buf, *gcep.key)
		if err != nil {
			return errors.New("Encrypt: " + err.Error())
		}
		buffer := bytes.NewBuffer(cipherBuf)
		_, err = io.Copy(w, buffer)
		if err != nil {
			return errors.New("Copy: " + err.Error())
		}
	}

	return nil
}

func (gcep *gcEndpoint) putObjectFile(fname string, ObjName string) error {
	f, err := os.Open(fname)
	r := io.Reader(f)

	obj := gcep.bucket.Object(ObjName)
	ctx := context.TODO()
	w := obj.NewWriter(ctx)
	defer w.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return errors.New("Copy " + err.Error())
	}

	return nil
}

func (gcep *gcEndpoint) objExists(objName string) (bool, error) {
	ctx := context.Background()

	q := &storage.Query{Prefix: objName}
	it := gcep.bucket.Objects(ctx, q)

	_, e := it.Next()

	if e != nil {
		if e != iterator.Done {
			return false, errors.New("Iterator: " + e.Error())
		}
		return false, nil
	}
	return true, nil
}

// Directory Metadata Handler
func gcMetadataHandler(gcep *gcEndpoint) {
	var base metadata.OffsetT

	defer gcep.mdwg.Done()

	mdmap := make(map[metadata.OffsetT]*metadata.Entry)

	f, e := ioutil.TempFile("", "Metadata_GC_")
	if e != nil {
		gcep.err = errors.New("metadata create " + e.Error())
		return
	}
	tmpFileName := f.Name()
	defer os.Remove(tmpFileName)

	// Put the header on the front of the file
	f.WriteString(metadata.Header() + "\n")

	baseEntry := metadata.NextEntry(gcep.baseReader, gcep.key)

	for newmde := range gcep.mdChan {
		mdmap[newmde.Offset()] = newmde

		mde, ok := mdmap[base]
		for ok {
			// ObjID is not set if clean and not object source
			if mde.ObjID == "" {
				if gcep.baseSnapName == "" {
					baseEntry.ObjID = metadata.ZeroHash
				} else {
					for ; baseEntry.Offset() != mde.Offset(); baseEntry = metadata.NextEntry(gcep.baseReader, gcep.key) {
						if baseEntry.Offset() == metadata.Infinity {
							// The volume grew
							baseEntry.ObjID = metadata.ZeroHash
							break
						}
					}
				}
				mde.ObjID = baseEntry.ObjID
			}
			mdeLine := mde.Encode(gcep.key)
			_, e = f.WriteString(mdeLine + "\n")
			if e != nil {
				f.Close()
				gcep.err = errors.New("write to metadata: " + e.Error())
				return
			}
			delete(mdmap, mde.Offset())
			base += metadata.ObjSize
			gcep.stats.Set(StatNameProgress, int64(base))
			mde, ok = mdmap[base]
		}
	}
	f.Close()

	incrSnapObjectName := metadata.SnapMetadataName(gcep.key,
		gcep.args.Domain,
		gcep.incrSnapName)
	e = gcep.putObjectFile(tmpFileName, incrSnapObjectName)
	if e != nil {
		gcep.err = errors.New("put of metadata for " + incrSnapObjectName + " " + e.Error())
	}
}
