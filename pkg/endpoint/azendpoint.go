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
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"path"
	"sync"
	"time"
)

// AzArgs is the set of arguments expected for an Azure copy endpoint
type AzArgs struct {
	BucketName       string
	Domain           string
	Base             string
	Incr             string
	PassPhrase       string
	StorageAccount   string
	StorageAccessKey string
}

type azEndpoint struct {
	args         *AzArgs              // Arguments passed in
	purpose      Purpose              // Source or Destination or Manipulation
	workerCount  uint                 // Number of workers expected (for scaling)
	bucketName   string               // The bucket in the object store
	baseSnapName string               // Base Snapshot Name
	incrSnapName string               // Incremental Snapshot Name
	baseReader   *csv.Reader          // Reader for Base Snapshot Metadata
	incrReader   *csv.Reader          // Reader for Incremental Snapshot Metadata
	mdChan       chan *metadata.Entry // Metadata Channel used for collecting Metadata changes
	mdwg         *sync.WaitGroup      // Waitgroup to wait for go routines to finish
	lunSize      int64                // Size of the lun
	stats        *stats.Stats         // Collector of Statistics
	containerURL azblob.ContainerURL  // Accessors to Azure
	key          *[]byte              // Encryption Key (nil if encryption not desired)
	id           string               // A unique Identifier
	err          error                // Background error to be reported
}

func (azep *azEndpoint) Setup() error {
	// break up the argument string
	// set the state
	var err error
	args := azep.args

	azep.bucketName = args.BucketName
	azep.baseSnapName = args.Base
	azep.incrSnapName = args.Incr
	rand.Seed(int64(os.Getpid()))
	azep.id = fmt.Sprintf("%x", rand.Uint64())

	if args.Domain == "" {
		return errors.New("Azure protection domain must be set")
	}

	if args.PassPhrase != "" {
		b := encrypt.PwToKey(args.PassPhrase, args.Domain, 32)
		azep.key = &b
	}

	// From the Azure portal, get your storage account name and key and set environment variables.
	accountName, accountKey := os.Getenv("AZURE_STORAGE_ACCOUNT"), os.Getenv("AZURE_STORAGE_ACCESS_KEY")
	if len(accountName) == 0 || len(accountKey) == 0 {
		return errors.New("Either the AZURE_STORAGE_ACCOUNT or AZURE_STORAGE_ACCESS_KEY environment variable is not set")
	}

	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return errors.New("Azure NewSharedKeyCredential Error: " + err.Error())
	}
	pl := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	URL, _ := url.Parse(
		fmt.Sprintf("https://%s.blob.core.windows.net/%s", accountName, azep.bucketName))

	azep.containerURL = azblob.NewContainerURL(*URL, pl)

	switch azep.purpose {
	case destination:
		if azep.baseSnapName != "" {
			baseSnapObjectName := metadata.SnapMetadataName(azep.key,
				azep.args.Domain,
				azep.baseSnapName)
			present, err := azep.objExists(baseSnapObjectName)
			if err != nil {
				return errors.New("Exists: " + err.Error())
			}
			if !present {
				return errors.New("Base Snapshot Not Present")
			}
			err = azep.getObjectFile(baseSnapObjectName, "/tmp/base_az_"+azep.id)
			if err != nil {
				return errors.New("Can't retrieve base snapshot " + err.Error())
			}
			azep.baseReader, err = metadata.NewMetaReader("/tmp/base_az_" + azep.id)
			if err != nil {
				return errors.New("Can't setup base snapshot reader " + err.Error())
			}
		}
		// We need a metadata handler for a destination
		azep.mdwg = &sync.WaitGroup{}
		azep.mdwg.Add(1)
		azep.mdChan = make(chan *metadata.Entry, azep.workerCount*2)
		go azMetadataHandler(azep)
	case source:
		if azep.baseSnapName == "" {
			azep.baseReader, err = metadata.NewMetaReader("/dev/null")
			if err != nil {
				return errors.New("Can't setup null snapshot reader " + err.Error())
			}
		} else {
			baseSnapObjectName := metadata.SnapMetadataName(azep.key, azep.args.Domain, azep.baseSnapName)
			err = azep.getObjectFile(baseSnapObjectName, "/tmp/base_az_"+azep.id)
			if err != nil {
				return errors.New("Can't retrieve base snapshot " + err.Error())
			}
			azep.baseReader, err = metadata.NewMetaReader("/tmp/base_az_" + azep.id)
			if err != nil {
				return errors.New("Can't setup base snapshot reader " + err.Error())
			}
		}
		incrSnapObjectName := metadata.SnapMetadataName(azep.key,
			azep.args.Domain,
			azep.incrSnapName)
		err = azep.getObjectFile(incrSnapObjectName, "/tmp/incr_az_"+azep.id)
		if err != nil {
			return errors.New("Can't retrieve incr snapshot " + err.Error())
		}

		size, err := metadata.SnapSize("/tmp/incr_az_"+azep.id, azep.key)
		if err != nil {
			return errors.New("getting lun size: " + err.Error())
		}

		azep.lunSize = int64(size)
		azep.incrReader, err = metadata.NewMetaReader("/tmp/incr_az_" + azep.id)
		if err != nil {
			return errors.New("setup incremental snapshot reader " + err.Error())
		}
		azep.stats.Set(StatNameSrcReadCount, 0)
	case manipulation:
		// Nothing to do
	default:
		return errors.New("Invalid Purpose for Dir Endpoint")
	}

	if azep.purpose == source || azep.purpose == destination {
		var baseName string
		if azep.baseSnapName == "" {
			baseName = "null"
		} else {
			baseName = azep.baseSnapName
		}

		fmt.Printf("%s Azure Endpoint on %s: %s, %s -> %s\n", azep.purpose.String(),
			azep.bucketName, azep.args.Domain, baseName, azep.incrSnapName)
	}
	return nil
}

// Could move this to metadata package
func (azep *azEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)
	maxSleepTime := time.Millisecond * 500

	// should make the meta chan internal
	baseMDE := metadata.NextEntry(azep.baseReader, azep.key)
	incrMDE := metadata.NextEntry(azep.incrReader, azep.key)

	for incrMDE.Offset() != metadata.Infinity {
		if shouldAbort() {
			return errors.New("Stopped due to abort condition")
		}
		var chunk *metadata.ChunkInfo

		chunk = nil

		if incrMDE.Offset() == baseMDE.Offset() {
			if incrMDE.ObjID == baseMDE.ObjID {
				// No Difference
				azep.stats.Add(StatNameDiffBytesUnchanged, metadata.ObjSize)
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  false,
				}
			} else {
				// Difference
				azep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  true,
				}
			}
			baseMDE = metadata.NextEntry(azep.baseReader, azep.key)
			incrMDE = metadata.NextEntry(azep.incrReader, azep.key)
		} else if incrMDE.Offset() < baseMDE.Offset() {
			// Difference
			azep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
			chunk = &metadata.ChunkInfo{
				Offset: incrMDE.Offset(),
				Length: metadata.ObjSize,
				Hash:   incrMDE.ObjID,
				Dirty:  true,
			}
			incrMDE = metadata.NextEntry(azep.incrReader, azep.key)
		} else if incrMDE.Offset() > baseMDE.Offset() {
			// How can this happen?
			// Since fixed Span Size not a diff
			baseMDE = metadata.NextEntry(azep.baseReader, azep.key)
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

func (azep *azEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if hint == "" {
		return errors.New("Hint Not Set")
	}

	if hint == metadata.ZeroHash {
		azep.stats.Add(StatNameReadZeroSkipped, metadata.ObjSize)
		for i := 0; i < metadata.ObjSize; i++ {
			buf[i] = 0
		}
		return nil
	}

	objName := metadata.NameToObjID(azep.key, azep.args.Domain, hint)

	n, e := azep.getObjectBytes(objName, buf)
	if e != nil {
		return errors.New("azure get object bytes: " + e.Error())
	}

	if n != metadata.ObjSize {
		return errors.New("azure fetch wrong data size")
	}

	// Verify the data read matches the expect checksum.
	hash := metadata.DataToHash(buf)
	if hash != hint {
		fmt.Printf("azure fetched data checksum doesn't match\n")
	}

	azep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)
	azep.stats.Inc(StatNameSrcReadCount)

	return nil
}

func (azep *azEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if hint == "" {
		hint = metadata.DataToHash(buf)
	}

	objName := metadata.NameToObjID(azep.key, azep.args.Domain, hint)

	azep.stats.Add(StatNameBytesPreDedup, metadata.ObjSize)

	present, err := azep.objExists(objName)
	if err != nil {
		return errors.New("Exists: " + err.Error())
	}
	if present {
		azep.stats.Add(StatNameDestBytesDedup, metadata.ObjSize)
		azep.mdChan <- metadata.NewEntry(offset, hint)
		return nil
	}

	err = azep.putObjectBytes(objName, buf)
	if err != nil {
		return errors.New("azure put bytes: " + err.Error())
	}

	azep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)

	azep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)

	azep.mdChan <- metadata.NewEntry(offset, hint)

	return err
}

func (azep *azEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	azep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	azep.mdChan <- metadata.NewEntry(offset, hint)
	return nil
}

func (azep *azEndpoint) GetIncrSnapName() string {
	return azep.incrSnapName
}

func (azep *azEndpoint) GetLunSize() int64 {
	return azep.lunSize
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (azep *azEndpoint) StoreData(name string, data []byte) error {
	objName := metadata.NameToObjID(azep.key, azep.args.Domain, name)

	if err := azep.putObjectBytes(objName, data); err != nil {
		azep.err = errors.New("put object: " + err.Error())
		return azep.err
	}

	return nil
}

// ResteiveData is a catalog read function
func (azep *azEndpoint) RetreiveData(name string, data []byte) (int, error) {
	var n int

	objName := metadata.NameToObjID(azep.key, azep.args.Domain, name)

	n, err := azep.getObjectBytes(objName, data)
	if err != nil {
		azep.err = errors.New("get object: " + err.Error())
		return 0, azep.err
	}

	return n, nil
}

type azListIteratorCtx struct {
	nextToken azblob.Marker
	entries   []string
	getMore   bool
	next      int
}

func (azep *azEndpoint) GetListNext(genericCtx interface{}) (string, interface{}, error) {
	maxEntriesPerCall := int32(8)
	var azctx *azListIteratorCtx

	if genericCtx == nil {
		azctx = &azListIteratorCtx{
			getMore:   true,
			nextToken: azblob.Marker{},
		}
	} else {
		azctx = genericCtx.(*azListIteratorCtx)
	}

	if azctx.next < len(azctx.entries) {
		azctx.next++
		return azctx.entries[azctx.next-1], azctx, nil
	}

	if azctx.getMore == false {
		return "", nil, nil // All done
	}

	ctx := context.Background()

	prefix := metadata.NameToObjID(azep.key, azep.args.Domain, "") + "/"

	listBlob, err := azep.containerURL.ListBlobsHierarchySegment(ctx, azctx.nextToken, "/",
		azblob.ListBlobsSegmentOptions{
			Prefix:     prefix,
			MaxResults: maxEntriesPerCall,
		})
	if err != nil {
		return "", nil, errors.New("AZ List: " + err.Error())
	}

	for _, blobInfo := range listBlob.Segment.BlobItems {
		azctx.entries = append(azctx.entries, metadata.DecryptObjName(path.Base(blobInfo.Name), azep.key))
	}
	azctx.next = 0

	azctx.nextToken = listBlob.NextMarker
	if listBlob.NextMarker.NotDone() {
		azctx.getMore = true
	} else {
		azctx.getMore = false
	}

	return azep.GetListNext(azctx)
}

func (azep *azEndpoint) Done(success bool) error {
	switch azep.purpose {
	case source:
		_ = os.Remove("/tmp/base_az_" + azep.id)
		_ = os.Remove("/tmp/incr_az_" + azep.id)
	case destination:
		// Close the mdChan to stop the metadata handler
		close(azep.mdChan)
		// Wait for it to finish
		azep.mdwg.Wait()
		_ = os.Remove("/tmp/base_az_" + azep.id)
	case manipulation:
		// Nothing to do
	default:
		fmt.Printf("Done on a bad Endpoint.")
	}
	return azep.err
}

func (azep *azEndpoint) getObjectBytes(objName string, buf []byte) (int, error) {
	var err error

	ctx := context.Background()
	blockBlobURL := azep.containerURL.NewBlockBlobURL(objName)

	var length int

	props, err := blockBlobURL.BlobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if err != nil {
		return 0, errors.New("get props: " + err.Error())
	}
	contentLength := props.ContentLength()

	if azep.key == nil {
		err = azblob.DownloadBlobToBuffer(ctx, blockBlobURL.BlobURL, int64(0), contentLength,
			buf, azblob.DownloadFromBlobOptions{
				BlockSize:   int64(1 * 1024 * 1024),
				Parallelism: uint16(2)})

		if err != nil {
			return 0, errors.New("download: " + err.Error())
		}
		length = int(contentLength)
	} else {
		ebuf := make([]byte, contentLength)

		err = azblob.DownloadBlobToBuffer(ctx, blockBlobURL.BlobURL, int64(0), contentLength,
			ebuf, azblob.DownloadFromBlobOptions{
				BlockSize:   int64(2 * 1024 * 1024),
				Parallelism: uint16(2)})
		if err != nil {
			return 0, errors.New("Download: " + err.Error())
		}

		dataBuf, err := encrypt.Decrypt(ebuf, *azep.key)
		if err != nil {
			return 0, errors.New("Decrypt: " + err.Error())
		}
		copy(buf, dataBuf)
		length = len(dataBuf)
	}

	return length, nil
}

func (azep *azEndpoint) getObjectFile(ObjName string, fname string) error {
	var err error

	file, err := os.Create(fname)
	if err != nil {
		return errors.New("Create " + err.Error())
	}
	defer file.Close()

	ctx := context.Background()

	blockBlobURL := azep.containerURL.NewBlockBlobURL(ObjName)

	err = azblob.DownloadBlobToFile(ctx, blockBlobURL.BlobURL, int64(0), int64(0),
		file, azblob.DownloadFromBlobOptions{})

	if err != nil {
		return errors.New("DownloadBlobToFile: " + err.Error())
	}

	return nil
}

func (azep *azEndpoint) putObjectBytes(objName string, buf []byte) error {
	var err error

	if azep.key == nil {
		ctx := context.Background()
		blobURL := azep.containerURL.NewBlockBlobURL(objName)

		_, err = azblob.UploadBufferToBlockBlob(ctx, buf,
			blobURL, azblob.UploadToBlockBlobOptions{
				BlockSize:   2 * 1024 * 1024,
				Parallelism: 16})
		if err != nil {
			return errors.New("Upload: " + err.Error())
		}
	} else {
		cipherBuf, err := encrypt.Encrypt(buf, *azep.key)
		if err != nil {
			return errors.New("Encrypt: " + err.Error())
		}

		ctx := context.Background()
		blobURL := azep.containerURL.NewBlockBlobURL(objName)

		_, err = azblob.UploadBufferToBlockBlob(ctx, cipherBuf,
			blobURL, azblob.UploadToBlockBlobOptions{
				BlockSize:   2 * 1024 * 1024,
				Parallelism: 16})
		if err != nil {
			return errors.New("Upload: " + err.Error())
		}
	}

	return nil
}

func (azep *azEndpoint) putObjectFile(fname string, ObjName string) error {

	file, err := os.Open(fname)
	if err != nil {
		return errors.New("Open: " + err.Error())
	}
	defer file.Close()

	ctx := context.Background() // This example uses a never-expiring context
	blobURL := azep.containerURL.NewBlockBlobURL(ObjName)

	_, err = azblob.UploadFileToBlockBlob(ctx, file, blobURL, azblob.UploadToBlockBlobOptions{
		BlockSize:   2 * 1024 * 1024,
		Parallelism: 16})

	if err != nil {
		return errors.New("Upload: " + err.Error())
	}

	return nil
}

func (azep *azEndpoint) objExists(objName string) (bool, error) {
	ctx := context.Background()

	res, err := azep.containerURL.ListBlobsFlatSegment(ctx, azblob.Marker{},
		azblob.ListBlobsSegmentOptions{Prefix: objName, MaxResults: 1})
	if err != nil {
		return false, errors.New("List: " + err.Error())
	}

	if len(res.Segment.BlobItems) == 1 {
		return true, nil
	}

	return false, nil
}

// Directory Metadata Handler
func azMetadataHandler(azep *azEndpoint) {
	var base metadata.OffsetT

	defer azep.mdwg.Done()

	mdmap := make(map[metadata.OffsetT]*metadata.Entry)

	f, e := ioutil.TempFile("", "Metadata_")
	if e != nil {
		azep.err = errors.New("Metadata: " + azep.incrSnapName + "Temp File Create: " + e.Error())
		return
	}
	tmpFileName := f.Name()
	defer os.Remove(tmpFileName)

	// Put the header on the front of the file
	f.WriteString(metadata.Header() + "\n")

	baseEntry := metadata.NextEntry(azep.baseReader, azep.key)

	for newmde := range azep.mdChan {
		mdmap[newmde.Offset()] = newmde

		mde, ok := mdmap[base]
		for ok {
			// ObjID is not set if clean and not object source
			if mde.ObjID == "" {
				if azep.baseSnapName == "" {
					baseEntry.ObjID = metadata.ZeroHash
				} else {
					for ; baseEntry.Offset() != mde.Offset(); baseEntry = metadata.NextEntry(azep.baseReader, azep.key) {
						if baseEntry.Offset() == metadata.Infinity {
							// The volume grew
							baseEntry.ObjID = metadata.ZeroHash
							break
						}
					}
				}
				mde.ObjID = baseEntry.ObjID
			}
			mdeLine := mde.Encode(azep.key)
			_, e = f.WriteString(mdeLine + "\n")
			if e != nil {
				f.Close()
				azep.err = errors.New("Write to metadata: " + azep.incrSnapName + e.Error())
				return
			}
			delete(mdmap, mde.Offset())
			base += metadata.ObjSize
			azep.stats.Set(StatNameProgress, int64(base))
			mde, ok = mdmap[base]
		}
	}
	f.Close()

	incrSnapObjectName := metadata.SnapMetadataName(azep.key,
		azep.args.Domain,
		azep.incrSnapName)
	e = azep.putObjectFile(tmpFileName, incrSnapObjectName)
	if e != nil {
		azep.err = errors.New("Put metadata file: " + azep.incrSnapName + e.Error())
	}
}
