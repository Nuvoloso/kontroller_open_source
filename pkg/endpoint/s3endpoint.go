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
	//"github.com/aws/aws-sdk-go-v2/aws"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/encrypt"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sync"
	"time"
)

// S3Args is the set of arguments expected for an AWS S3 copy endpoint
type S3Args struct {
	BucketName      string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Domain          string
	Base            string
	Incr            string
	PassPhrase      string
}

type s3Endpoint struct {
	args         *S3Args              // Arguments passed in
	purpose      Purpose              // Source or Destination
	workerCount  uint                 // Number of workers (for scale)
	bucket       string               // The bucket in the object store
	baseSnapName string               // Base Snapshot Name
	incrSnapName string               // Incremental Snapshot Name
	baseReader   *csv.Reader          // Reader for Base Snapshot Metadata
	incrReader   *csv.Reader          // Reader for Incremental Snapshot Metadata
	mdChan       chan *metadata.Entry // Metadata Channel used for collecting Metadata changes
	mdwg         *sync.WaitGroup      // Waitgroup to wait for go routines to finish
	lunSize      int64                // Size of the lun
	svc          *s3.S3               // Connection to S3
	stats        *stats.Stats         // Collector of Statistics
	key          *[]byte              // Encryption Key (nil if encryption not desired)
	id           string               // A unique Identifier
	err          error                // Error to return
}

const (
	// maxS3ListEntriesPerCall is how many entries per list call
	// maxS3ListEntriesPerCall int = 1000
	maxS3ListEntriesPerCall int64 = 8
)

func (s3ep *s3Endpoint) Setup() error {
	var err error

	args := s3ep.args

	s3ep.bucket = args.BucketName
	s3ep.baseSnapName = args.Base
	s3ep.incrSnapName = args.Incr
	rand.Seed(int64(os.Getpid()))
	s3ep.id = fmt.Sprintf("%x", rand.Uint64())

	if args.Domain == "" {
		return errors.New("S3 protection domain must be set")
	}

	if args.PassPhrase != "" {
		b := encrypt.PwToKey(args.PassPhrase, args.Domain, 32)
		s3ep.key = &b
	}

	cfg, err := external.LoadDefaultAWSConfig(
		external.WithCredentialsValue(aws.Credentials{
			AccessKeyID: args.AccessKeyID, SecretAccessKey: args.SecretAccessKey,
			SessionToken: "", Source: "JSON copy argument file",
		}),
	)
	if err != nil {
		return errors.New("LoadDefaultConfig: " + err.Error())
	}

	cfg.Region = args.Region

	s3ep.svc = s3.New(cfg)

	switch s3ep.purpose {
	case destination:
		if s3ep.baseSnapName != "" {
			baseSnapObjectName := metadata.SnapMetadataName(
				s3ep.key, s3ep.args.Domain, s3ep.baseSnapName)
			present, err := s3ep.objExists(baseSnapObjectName)
			if err != nil {
				return errors.New("base existence check: " + err.Error())
			}
			if !present {
				return errors.New("base snapshot not present")
			}
			err = s3ep.getObjectFile(baseSnapObjectName, "/tmp/base_s3_"+s3ep.id)
			if err != nil {
				return errors.New("retrieve base snapshot: " + err.Error())
			}
			s3ep.baseReader, err = metadata.NewMetaReader("/tmp/base_s3_" + s3ep.id)
			if err != nil {
				return errors.New("setup base snapshot reader: " + err.Error())
			}
		}

		// TODO use the read volume name instead of BoGuSv
		incrSnapObjectName := metadata.SnapMetadataName(
			s3ep.key, s3ep.args.Domain, s3ep.incrSnapName)

		present, err := s3ep.objExists(incrSnapObjectName)
		if err != nil {
			return errors.New("incr existence check: " + err.Error())
		}

		if present {
			return errors.New("incremental snapshot already present")
		}

		// We need a metadata handler for a destination
		s3ep.mdwg = &sync.WaitGroup{}
		s3ep.mdwg.Add(1)
		s3ep.mdChan = make(chan *metadata.Entry, s3ep.workerCount*2)
		go s3MetadataHandler(s3ep)
	case source:
		if s3ep.baseSnapName == "" {
			s3ep.baseReader, err = metadata.NewMetaReader("/dev/null")
			if err != nil {
				return errors.New("setup null snapshot reader: " + err.Error())
			}
		} else {
			baseSnapObjectName := metadata.SnapMetadataName(
				s3ep.key, s3ep.args.Domain, s3ep.baseSnapName)
			err = s3ep.getObjectFile(baseSnapObjectName, "/tmp/base_s3_"+s3ep.id)
			if err != nil {
				return errors.New("retrieve base snapshot: " + err.Error())
			}
			s3ep.baseReader, err = metadata.NewMetaReader("/tmp/base_s3_" + s3ep.id)
			if err != nil {
				return errors.New("setup base snapshot reader: " + err.Error())
			}
		}

		incrSnapObjectName := metadata.SnapMetadataName(
			s3ep.key, s3ep.args.Domain, s3ep.incrSnapName)
		err = s3ep.getObjectFile(incrSnapObjectName, "/tmp/incr_s3_"+s3ep.id)
		if err != nil {
			return errors.New("retrieve incr snapshot: " + err.Error())
		}

		size, err := metadata.SnapSize("/tmp/incr_s3_"+s3ep.id, s3ep.key)
		if err != nil {
			return errors.New("getting lun size: " + err.Error())
		}

		s3ep.lunSize = int64(size)
		s3ep.incrReader, err = metadata.NewMetaReader("/tmp/incr_s3_" + s3ep.id)
		if err != nil {
			return errors.New("setup incremental snapshot reader: " + err.Error())
		}
		s3ep.stats.Set(StatNameSrcReadCount, 0)

	case manipulation:
		// Nothing to do
	default:
		return errors.New("invalid purpose for S3 endpoint")
	}

	if s3ep.purpose == source || s3ep.purpose == destination {
		var baseName string
		if s3ep.baseSnapName == "" {
			baseName = "null"
		} else {
			baseName = s3ep.baseSnapName
		}

		fmt.Printf("%s S3 Endpoint on %s: %s -> %s\n", s3ep.purpose.String(), s3ep.bucket,
			baseName, s3ep.incrSnapName)
	}

	return nil
}

// Could move this to metadata package
func (s3ep *s3Endpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)
	maxSleepTime := time.Millisecond * 500

	// should make the meta chan internal
	baseMDE := metadata.NextEntry(s3ep.baseReader, s3ep.key)
	incrMDE := metadata.NextEntry(s3ep.incrReader, s3ep.key)

	for incrMDE.Offset() != metadata.Infinity {
		if shouldAbort() {
			return errors.New("stopped due to abort condition")
		}
		var chunk *metadata.ChunkInfo
		chunk = nil

		if incrMDE.Offset() == baseMDE.Offset() {
			if incrMDE.ObjID == baseMDE.ObjID {
				// No Difference
				s3ep.stats.Add(StatNameDiffBytesUnchanged, metadata.ObjSize)
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  false,
				}
			} else {
				// Difference
				s3ep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
				chunk = &metadata.ChunkInfo{
					Offset: incrMDE.Offset(),
					Length: metadata.ObjSize,
					Hash:   incrMDE.ObjID,
					Dirty:  true,
				}
			}
			baseMDE = metadata.NextEntry(s3ep.baseReader, s3ep.key)
			incrMDE = metadata.NextEntry(s3ep.incrReader, s3ep.key)
		} else if incrMDE.Offset() < baseMDE.Offset() {
			// Difference
			s3ep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)
			chunk = &metadata.ChunkInfo{
				Offset: incrMDE.Offset(),
				Length: metadata.ObjSize,
				Hash:   incrMDE.ObjID,
				Dirty:  true,
			}
			incrMDE = metadata.NextEntry(s3ep.incrReader, s3ep.key)
		} else if incrMDE.Offset() > baseMDE.Offset() {
			// How can this happen?
			// Since fixed Span Size not a diff
			baseMDE = metadata.NextEntry(s3ep.baseReader, s3ep.key)
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

func (s3ep *s3Endpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if hint == "" {
		return errors.New("hint Not Set")
	}

	if hint == metadata.ZeroHash {
		s3ep.stats.Add(StatNameReadZeroSkipped, metadata.ObjSize)
		for i := 0; i < metadata.ObjSize; i++ {
			buf[i] = 0
		}
		return nil
	}

	objName := metadata.NameToObjID(s3ep.key, s3ep.args.Domain, hint)

	n, e := s3ep.getObjectBytes(objName, buf)
	if e != nil {
		return errors.New("get object bytes: " + e.Error())
	}

	if n != metadata.ObjSize {
		return errors.New("size object not correct")
	}

	s3ep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)
	s3ep.stats.Inc(StatNameSrcReadCount)

	return nil
}

func (s3ep *s3Endpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if hint == "" {
		hint = metadata.DataToHash(buf)
	}

	objName := metadata.NameToObjID(s3ep.key, s3ep.args.Domain, hint)

	s3ep.stats.Add(StatNameBytesPreDedup, metadata.ObjSize)

	present, err := s3ep.objExists(objName)
	if err != nil {
		return errors.New("fillat existence check: " + err.Error())
	}
	if present {
		s3ep.stats.Add(StatNameDestBytesDedup, metadata.ObjSize)
		s3ep.mdChan <- metadata.NewEntry(offset, hint)
		return nil
	}

	err = s3ep.putObjectBytes(objName, buf)
	if err != nil {
		return errors.New("put object bytes: " + err.Error())
	}
	s3ep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)

	s3ep.mdChan <- metadata.NewEntry(offset, hint)

	return nil
}

func (s3ep *s3Endpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	s3ep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	s3ep.mdChan <- metadata.NewEntry(offset, hint)
	return nil
}

func (s3ep *s3Endpoint) GetIncrSnapName() string {
	return s3ep.incrSnapName
}

func (s3ep *s3Endpoint) GetLunSize() int64 {
	return s3ep.lunSize
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (s3ep *s3Endpoint) StoreData(name string, data []byte) error {
	objName := metadata.NameToObjID(s3ep.key, s3ep.args.Domain, name)

	if err := s3ep.putObjectBytes(objName, data); err != nil {
		s3ep.err = errors.New("put object: " + err.Error())
		return s3ep.err
	}

	return nil
}

// ResteiveData is a catalog read function
func (s3ep *s3Endpoint) RetreiveData(name string, data []byte) (int, error) {
	var n int

	objName := metadata.NameToObjID(s3ep.key, s3ep.args.Domain, name)

	n, err := s3ep.getObjectBytes(objName, data)
	if err != nil {
		s3ep.err = errors.New("get object: " + err.Error())
		return 0, s3ep.err
	}

	return n, nil
}

type s3ListIteratorCtx struct {
	nextToken string
	entries   []string
	getMore   bool
	next      int
}

// GetList returns the decrypted names of objects in the domain
// one at a time.
func (s3ep *s3Endpoint) GetListNext(ctx interface{}) (string, interface{}, error) {
	var s3ctx *s3ListIteratorCtx

	if ctx == nil {
		s3ctx = &s3ListIteratorCtx{getMore: true}
	} else {
		s3ctx = ctx.(*s3ListIteratorCtx)
	}

	if s3ctx.next < len(s3ctx.entries) {
		// Give a stored entry
		s3ctx.next++
		return s3ctx.entries[s3ctx.next-1], s3ctx, nil
	}

	if s3ctx.getMore == false {
		// There are no more
		return "", nil, nil
	}
	// Get more from the bucket

	prefix := metadata.NameToObjID(s3ep.key, s3ep.args.Domain, "") + "/"

	var maxCount = maxS3ListEntriesPerCall
	params := &s3.ListObjectsInput{
		Bucket:    &s3ep.bucket,
		Delimiter: aws.String("/"),
		Prefix:    &prefix,
		MaxKeys:   &maxCount,
		Marker:    &s3ctx.nextToken,
	}

	if s3ctx.nextToken == "" {
		params.Marker = nil
	}

	req := s3ep.svc.ListObjectsRequest(params)

	output, e := req.Send()
	if e != nil {
		return "", nil, errors.New("GetList: " + e.Error())
	}
	s3ctx.entries = []string{}

	for _, o := range output.Contents {
		s3ctx.entries = append(s3ctx.entries, metadata.DecryptObjName(path.Base(*o.Key), s3ep.key))
	}
	s3ctx.next = 0

	if *output.IsTruncated {
		s3ctx.nextToken = *output.NextMarker
		s3ctx.getMore = true
	} else {
		s3ctx.getMore = false
		s3ctx.nextToken = ""
	}

	return s3ep.GetListNext(s3ctx)
}

func (s3ep *s3Endpoint) Done(success bool) error {
	switch s3ep.purpose {
	case source:
		_ = os.Remove("/tmp/incr_s3_" + s3ep.id)
		_ = os.Remove("/tmp/base_s3_" + s3ep.id)
	case destination:
		// Close the mdChan to stop the metadata handler
		close(s3ep.mdChan)
		// Wait for it to finish
		s3ep.mdwg.Wait()
		_ = os.Remove("/tmp/base_s3_" + s3ep.id)
	case manipulation:
		// Nothing to do
	default:
		fmt.Printf("Done on a bad Endpoint.")
	}

	return s3ep.err
}

func (s3ep *s3Endpoint) getObjectBytes(objName string, buf []byte) (int, error) {
	params := &s3.GetObjectInput{
		Bucket: &s3ep.bucket,
		Key:    &objName,
	}

	req := s3ep.svc.GetObjectRequest(params)

	output, err := req.Send()
	if err != nil {
		return 0, errors.New("get object bytes send: " + err.Error())
	}

	dbuf, err := ioutil.ReadAll(output.Body)
	if err != nil {
		return 0, errors.New("s3 read all object: " + err.Error())
	}

	var length int

	if s3ep.key == nil {
		copy(buf, dbuf[:len(dbuf)])
		length = len(dbuf)
	} else {
		dataBuf, err := encrypt.Decrypt(dbuf, *s3ep.key)
		if err != nil {
			return 0, errors.New("decrypt: " + err.Error())
		}
		copy(buf, dataBuf[:len(dataBuf)])
		length = len(dataBuf)
	}

	return length, nil
}

func (s3ep *s3Endpoint) getObjectFile(objName string, fname string) error {
	params := &s3.GetObjectInput{
		Bucket: &s3ep.bucket,
		Key:    &objName,
		//SSECustomerAlgorithm: "AES256",
		//SSECustomerKey: "Federwisch",
	}

	req := s3ep.svc.GetObjectRequest(params)

	output, e := req.Send()
	if e != nil {
		return errors.New("getObjectFile send: " + e.Error())
	}

	var f *os.File

	f, e = os.Create(fname)
	if e != nil {
		return errors.New("getObjectFile open: " + e.Error())
	}
	_, e = io.Copy(f, output.Body)
	if e != nil {
		return errors.New("getObjectFile copy: " + e.Error())
	}

	f.Close()

	return e
}

func (s3ep *s3Endpoint) putObjectBytes(objName string, buf []byte) error {
	var params *s3.PutObjectInput

	if s3ep.key == nil {
		params = &s3.PutObjectInput{
			Bucket: &s3ep.bucket,
			Key:    &objName,
			Body:   bytes.NewReader(buf),
		}
	} else {
		cipherBuf, err := encrypt.Encrypt(buf, *s3ep.key)
		if err != nil {
			return errors.New("put object bytes encrypt: " + err.Error())
		}

		// shouldn't need the length
		length := len(cipherBuf)

		params = &s3.PutObjectInput{
			Bucket: &s3ep.bucket,
			Key:    &objName,
			Body:   bytes.NewReader(cipherBuf[:length]),
		}
	}

	req := s3ep.svc.PutObjectRequest(params)

	_, e := req.Send()
	if e != nil {
		return errors.New("fillat send: " + e.Error())
	}
	return nil
}

func (s3ep *s3Endpoint) putObjectFile(fname string, objName string) error {

	f, e := os.Open(fname)

	params := &s3.PutObjectInput{
		Bucket: &s3ep.bucket,
		Key:    &objName,
		Body:   f,
	}
	// encrypt this?

	req := s3ep.svc.PutObjectRequest(params)

	_, e = req.Send()
	if e != nil {
		return errors.New("putObjectFile send: " + e.Error())
	}
	return nil
}

func (s3ep *s3Endpoint) objExists(objName string) (bool, error) {
	var maxCount int64 = 1

	params := &s3.ListObjectsInput{
		Bucket:  &s3ep.bucket,
		Prefix:  &objName,
		MaxKeys: &maxCount,
	}

	req := s3ep.svc.ListObjectsRequest(params)

	output, e := req.Send()
	if e != nil {
		return false, errors.New("objexists list: " + e.Error())
	}

	for _, o := range output.Contents {
		if *o.Key == objName {
			return true, nil
		}
	}

	return false, nil
}

// s3MetadataHandler collects information from the other modules and builds
// the metadata file for the snapshot.
func s3MetadataHandler(s3ep *s3Endpoint) {
	var base metadata.OffsetT

	defer s3ep.mdwg.Done()

	mdmap := make(map[metadata.OffsetT]*metadata.Entry)

	f, err := ioutil.TempFile("", "Metadata_S3_")
	if err != nil {
		s3ep.err = errors.New("s3metadata tempfile: " + s3ep.incrSnapName + " " + err.Error())
		return
	}
	tmpFileName := f.Name()
	defer os.Remove(tmpFileName)

	// Put the header on the front of the file
	f.WriteString(metadata.Header() + "\n")

	baseEntry := metadata.NextEntry(s3ep.baseReader, s3ep.key)

	for newmde := range s3ep.mdChan {
		mdmap[newmde.Offset()] = newmde

		mde, ok := mdmap[base]
		for ok {
			// ObjID is not set if clean and not object source
			if mde.ObjID == "" {
				if s3ep.baseSnapName == "" {
					baseEntry.ObjID = metadata.ZeroHash
				} else {
					for ; baseEntry.Offset() != mde.Offset(); baseEntry = metadata.NextEntry(s3ep.baseReader, s3ep.key) {

						if baseEntry.Offset() == metadata.Infinity {
							// The volume grew
							baseEntry.ObjID = metadata.ZeroHash
							break
						}
					}
				}
				mde.ObjID = baseEntry.ObjID
			}
			mdeLine := mde.Encode(s3ep.key)
			_, err = f.WriteString(mdeLine + "\n")
			if err != nil {
				f.Close()
				s3ep.err = errors.New("s3 metadata line: " + err.Error())
				return
			}
			delete(mdmap, mde.Offset())
			base += metadata.ObjSize
			s3ep.stats.Set(StatNameProgress, int64(base))
			mde, ok = mdmap[base]
		}
	}
	f.Close()

	incrSnapObjectName := metadata.SnapMetadataName(
		s3ep.key, s3ep.args.Domain, s3ep.incrSnapName)
	err = s3ep.putObjectFile(tmpFileName, incrSnapObjectName)
	if err != nil {
		s3ep.err = errors.New("write metadata: " + s3ep.incrSnapName + " " + err.Error())
		return
	}
}
