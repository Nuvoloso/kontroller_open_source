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
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

// DirArgs is the set of arguments expected for an Directory copy endpoint
type DirArgs struct {
	Directory  string
	Domain     string
	Base       string
	Incr       string
	PassPhrase string
}

type dirEndpoint struct {
	args          *DirArgs
	purpose       Purpose
	workerCount   uint
	baseSnapName  string
	incrSnapName  string
	baseDirectory string
	baseReader    *csv.Reader
	incrReader    *csv.Reader
	mdChan        chan *metadata.Entry
	mdwg          *sync.WaitGroup
	lunSize       int64
	stats         *stats.Stats
	key           *[]byte
	id            string
	err           error
}

func (dep *dirEndpoint) Setup() error {
	var err error
	args := dep.args

	dep.baseDirectory = args.Directory
	dep.baseSnapName = args.Base
	dep.incrSnapName = args.Incr
	rand.Seed(int64(os.Getpid()))
	dep.id = fmt.Sprintf("%x", rand.Uint64())

	if args.Domain == "" {
		return errors.New("directory protection domain must be set")
	}

	if args.PassPhrase != "" {
		b := encrypt.PwToKey(args.PassPhrase, args.Domain, 32)
		dep.key = &b
	}

	switch dep.purpose {
	case destination:
		// make sure the directory structure for data is present
		err = createDirs(dep.baseDirectory + "/" + metadata.NameToObjID(dep.key, dep.args.Domain, "X"))
		if err != nil {
			return errors.New("Failed base directory create " + err.Error())
		}
		if dep.baseSnapName != "" {
			baseSnapObjectName := metadata.SnapMetadataName(dep.key,
				dep.args.Domain,
				dep.baseSnapName)
			exists := dep.objExists(baseSnapObjectName)
			if !exists {
				return errors.New("Base Snapshot Not Present: " + dep.baseSnapName + " " + baseSnapObjectName)
			}

			// the base metadata is needed when clean entries need filling
			dep.baseReader, err = metadata.NewMetaReader(dep.baseDirectory + "/" + baseSnapObjectName)
			if err != nil {
				return errors.New("setup base snapshot reader " + err.Error())
			}
		}

		dep.mdwg = &sync.WaitGroup{}
		dep.mdwg.Add(1)
		dep.mdChan = make(chan *metadata.Entry, dep.workerCount*2)
		go dirMetadataHandler(dep)
	case source:
		if dep.baseSnapName == "" {
			dep.baseReader, err = metadata.NewMetaReader("/dev/null")
			if err != nil {
				return errors.New("setup null snapshot reader " + err.Error())
			}
		} else {
			baseSnapObjectName := metadata.SnapMetadataName(dep.key,
				dep.args.Domain,
				dep.baseSnapName)
			dep.baseReader, err = metadata.NewMetaReader(dep.baseDirectory + "/" + baseSnapObjectName)
			if err != nil {
				return errors.New("setup base snapshot reader " + err.Error())
			}
		}
		incrSnapObjectName := metadata.SnapMetadataName(dep.key,
			dep.args.Domain,
			dep.incrSnapName)

		size, err := metadata.SnapSize(dep.baseDirectory+"/"+incrSnapObjectName, dep.key)
		if err != nil {
			return errors.New("getting lun size: " + err.Error())
		}

		dep.lunSize = int64(size)
		dep.incrReader, err = metadata.NewMetaReader(dep.baseDirectory + "/" + incrSnapObjectName)
		if err != nil {
			return errors.New("setup incremental snapshot reader " + err.Error())
		}
		dep.stats.Set(StatNameSrcReadCount, 0)
	case manipulation:
		// Nothing to do
	default:
		return errors.New("Invalid Purpose for Dir Endpoint")
	}

	if dep.purpose == source || dep.purpose == destination {
		var baseName string
		if dep.baseSnapName == "" {
			baseName = "null"
		} else {
			baseName = dep.baseSnapName
		}

		fmt.Printf("%s Directory Endpoint on %s: %s %s -> %s\n", dep.purpose.String(),
			dep.baseDirectory, dep.args.Domain, baseName, dep.incrSnapName)
	}

	return nil
}

func (dep *dirEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)
	maxSleepTime := time.Millisecond * 500

	// should make the meta chan internal
	baseMDE := metadata.NextEntry(dep.baseReader, dep.key)
	incrMDE := metadata.NextEntry(dep.incrReader, dep.key)

	for incrMDE.Offset() != metadata.Infinity {
		var chunk *metadata.ChunkInfo

		chunk = nil

		if shouldAbort() {
			return errors.New("Stopped due to abort condition")
		}
		if incrMDE.Offset() == baseMDE.Offset() {
			if incrMDE.ObjID == baseMDE.ObjID {
				dep.stats.Add(StatNameDiffBytesUnchanged, metadata.ObjSize)
				// No Difference
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  false,
				}
			} else {
				dep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
				// Difference
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  true,
				}
			}
			baseMDE = metadata.NextEntry(dep.baseReader, dep.key)
			incrMDE = metadata.NextEntry(dep.incrReader, dep.key)
		} else if incrMDE.Offset() < baseMDE.Offset() {
			dep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
			// Difference
			chunk = &metadata.ChunkInfo{
				Offset: incrMDE.Offset(),
				Length: metadata.ObjSize,
				Hash:   incrMDE.ObjID,
				Dirty:  true,
			}
			incrMDE = metadata.NextEntry(dep.incrReader, dep.key)
		} else if incrMDE.Offset() > baseMDE.Offset() {
			// How can this happen?
			// Since fixed Span Size not a diff
			baseMDE = metadata.NextEntry(dep.baseReader, dep.key)
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

func (dep *dirEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	// hint is the ObjID, just fetch that.
	// Otherwise we would need to look through the metadata to find the object.

	if hint == "" {
		return errors.New("Hint Not Set")
	}

	if hint == metadata.ZeroHash {
		dep.stats.Add(StatNameReadZeroSkipped, metadata.ObjSize)
		for i := 0; i < metadata.ObjSize; i++ {
			buf[i] = 0
		}
		return nil
	}

	_, err := dep.getObjectBytes(metadata.NameToObjID(dep.key, dep.args.Domain, hint), buf)
	if err != nil {
		return errors.New("get object bytes: " + err.Error())
	}

	dep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)
	dep.stats.Inc(StatNameSrcReadCount)

	return nil
}

func (dep *dirEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	var e error

	if hint == "" {
		hint = metadata.DataToHash(buf)
	}
	objName := metadata.NameToObjID(dep.key, dep.args.Domain, hint)

	dep.stats.Add(StatNameBytesPreDedup, metadata.ObjSize)

	exists := dep.objExists(objName)
	if exists {
		dep.stats.Add(StatNameDestBytesDedup, metadata.ObjSize)
		dep.mdChan <- metadata.NewEntry(offset, hint)
		return nil
	}

	e = dep.putObjectBytes(objName, buf)
	if e != nil {
		return errors.New("fillat: " + e.Error())
	}

	dep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)

	dep.mdChan <- metadata.NewEntry(offset, hint)
	return nil
}

func (dep *dirEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	dep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	dep.mdChan <- metadata.NewEntry(offset, hint)
	return nil
}

func (dep *dirEndpoint) GetIncrSnapName() string {
	return dep.incrSnapName
}

func (dep *dirEndpoint) GetLunSize() int64 {
	return dep.lunSize
}

func (dep *dirEndpoint) Done(success bool) error {
	switch dep.purpose {
	case source:
		// dep.baseReader.Close() or close the file?
		// dep.incrReader.Close() or close the file?
	case destination:
		// Close the mdChan to stop the metadata handler
		close(dep.mdChan)
		// Wait for it to finish
		dep.mdwg.Wait()
	case manipulation:
		// Nothing to do
	default:
		fmt.Printf("Done on a bad Endpoint.")
	}
	return dep.err
}

// Make sure the directory for a particular file exists
func createDirs(path string) error {
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return errors.New("MkdirAll: " + err.Error())
	}

	return nil
}

// Does the object exist
func (dep *dirEndpoint) objExists(objName string) bool {
	fileName := dep.baseDirectory + "/" + objName

	_, e := os.Stat(fileName)
	return !os.IsNotExist(e)
}

func (dep *dirEndpoint) getObjectBytes(objName string, buf []byte) (int, error) {
	o, e := os.Open(dep.baseDirectory + "/" + objName)
	if e != nil {
		return 0, errors.New(e.Error())
	}
	defer o.Close()

	if dep.key == nil {
		// read in the object
		dbuf, err := ioutil.ReadAll(o)
		if err != nil {
			return 0, errors.New("dir get object: " + e.Error())
		}
		copy(buf, dbuf[:len(dbuf)])
		return len(dbuf), nil
	}

	ebuf := make([]byte, metadata.ObjSize+1024)

	r := io.LimitReader(o, metadata.ObjSize+1024)
	n, err := r.Read(ebuf)

	if err != nil && err != io.EOF {
		return 0, errors.New("file read: " + e.Error())
	}

	dataBuf, err := encrypt.Decrypt(ebuf[:n], *dep.key)
	if err != nil {
		return 0, errors.New("decrypt: " + err.Error())
	}
	n = len(dataBuf)
	copy(buf, dataBuf[:n])
	return n, nil
}

func (dep *dirEndpoint) putObjectBytes(objName string, buf []byte) error {
	bufLen := len(buf)

	filePath := dep.baseDirectory + "/" + objName
	if e := os.MkdirAll(path.Dir(filePath), 0755); e != nil {
		return errors.New("MkdirAll " + e.Error())
	}

	obj, e := os.Create(filePath)
	if e != nil {
		return errors.New("Create " + e.Error())
	}
	defer obj.Close()

	// This should be changed to just use a pointer
	// to the buffer and encrypt if needed.  I'll do
	// that soon but just doing it very separately until
	// it works.
	var nbytes int

	if dep.key == nil {
		for totalBytes := 0; totalBytes < bufLen; {
			nbytes, e = obj.Write(buf[totalBytes:bufLen])
			if e != nil {
				return errors.New("Write: " + e.Error())
			}
			totalBytes += nbytes
		}
	} else {
		cipherBuf, err := encrypt.Encrypt(buf, *dep.key)
		if err != nil {
			return errors.New("Encrypt: " + e.Error())
		}

		length := len(cipherBuf)

		for totalBytes := 0; totalBytes < length; totalBytes += nbytes {
			nbytes, e = obj.Write(cipherBuf[totalBytes:length])
			if e != nil {
				return errors.New("Write: " + e.Error())
			}
		}
	}
	return nil
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (dep *dirEndpoint) StoreData(name string, data []byte) error {
	objName := metadata.NameToObjID(dep.key, dep.args.Domain, name)

	if err := dep.putObjectBytes(objName, data); err != nil {
		dep.err = errors.New("put object: " + err.Error())
		return dep.err
	}

	return nil
}

// ResteiveData is a catalog read function
func (dep *dirEndpoint) RetreiveData(name string, data []byte) (int, error) {
	var n int

	objName := metadata.NameToObjID(dep.key, dep.args.Domain, name)

	n, err := dep.getObjectBytes(objName, data)
	if err != nil {
		dep.err = errors.New("get object: " + err.Error())
		return 0, dep.err
	}

	return n, nil
}

type dirListIteratorCtx struct {
	entries []string
	next    int
}

func (dep *dirEndpoint) GetListNext(ctx interface{}) (string, interface{}, error) {
	var dirctx *dirListIteratorCtx

	if ctx == nil {
		dirName := dep.baseDirectory + "/" + metadata.NameToObjID(dep.key, dep.args.Domain, "") + "/"

		dirFile, err := os.Open(dirName)
		if err != nil {
			dep.err = errors.New("dir list open: " + err.Error())
			return "", nil, dep.err
		}
		defer dirFile.Close()

		dirNames, err := dirFile.Readdirnames(0)
		if err != nil {
			dep.err = errors.New("dir list readdirnames: " + err.Error())
			return "", nil, dep.err
		}

		dirctx = &dirListIteratorCtx{}

		for _, thisone := range dirNames {
			dirctx.entries = append(dirctx.entries, metadata.DecryptObjName(thisone, dep.key))
		}
	} else {
		dirctx = ctx.(*dirListIteratorCtx)
	}

	if dirctx.next == len(dirctx.entries) {
		return "", nil, nil // All done
	}
	dirctx.next++
	return dirctx.entries[dirctx.next-1], dirctx, nil
}

// Directory Metadata Handler
func dirMetadataHandler(dep *dirEndpoint) {
	var base metadata.OffsetT

	incrSnapObjectName := dep.baseDirectory + "/" +
		metadata.SnapMetadataName(dep.key, dep.args.Domain, dep.incrSnapName)

	defer dep.mdwg.Done()

	mdmap := make(map[metadata.OffsetT]*metadata.Entry)

	e := createDirs(incrSnapObjectName)
	if e != nil {
		dep.err = errors.New("Creating Directories for " + dep.incrSnapName + "Error: " + e.Error())
		return
	}

	f, e := ioutil.TempFile("", "Metadata_")
	if e != nil {
		dep.err = errors.New("Metadata: " + dep.incrSnapName + " Error: " + e.Error())
		return
	}
	tmpFileName := f.Name()
	defer os.Remove(tmpFileName)

	// Put the header on the front of the file
	f.WriteString(metadata.Header() + "\n")

	baseEntry := metadata.NextEntry(dep.baseReader, dep.key)

	for newmde := range dep.mdChan {
		mdmap[newmde.Offset()] = newmde

		mde, ok := mdmap[base]
		for ok {
			// ObjID is not set if clean and not object source
			if mde.ObjID == "" {
				if dep.baseSnapName == "" {
					baseEntry.ObjID = metadata.ZeroHash
				} else {
					for ; baseEntry.Offset() != mde.Offset(); baseEntry = metadata.NextEntry(dep.baseReader, dep.key) {
						if baseEntry.Offset() == metadata.Infinity {
							// The volume grew
							baseEntry.ObjID = metadata.ZeroHash
						}
					}
				}
				mde.ObjID = baseEntry.ObjID
			}
			mdeLine := mde.Encode(dep.key)
			_, e = f.WriteString(mdeLine + "\n")
			if e != nil {
				f.Close()
				dep.err = errors.New("Write to metadata: " + dep.incrSnapName + " Error: " + e.Error())
				return
			}
			delete(mdmap, mde.Offset())
			base += metadata.ObjSize
			dep.stats.Set(StatNameProgress, int64(base))
			mde, ok = mdmap[base]
		}
	}
	f.Close()

	e = os.Rename(tmpFileName, incrSnapObjectName)
	if e != nil {
		fmt.Printf("Put of metadata file failed: File: %s to %s Error: %s\n",
			tmpFileName, incrSnapObjectName, e.Error())
	}
}
