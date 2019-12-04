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
	"errors"
	"fmt"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"io"
	"os"
	"time"
)

// FileArgs is the set of arguments expected for an file copy endpoint
type FileArgs struct {
	FileName      string
	DestPreZeroed bool
}

type fileEndpoint struct {
	args         *FileArgs
	purpose      Purpose
	baseSnapName string
	incrSnapName string
	lun          *os.File
	lunSize      int64
	stats        *stats.Stats
}

func (fep *fileEndpoint) Setup() error {
	var err error
	args := fep.args

	fi, e := os.Stat(args.FileName)
	if e != nil {
		return errors.New(e.Error())
	}
	fep.lunSize = fi.Size()

	switch fep.purpose {
	case destination:
		fep.lun, err = os.OpenFile(args.FileName, os.O_WRONLY, 0755)
		if err != nil {
			return err
		}
	case source:
		fep.lun, err = os.Open(args.FileName)
		if err != nil {
			return err
		}
		fep.stats.Set(StatNameSrcReadCount, 0)
	case manipulation:
		// Nothing to do
	default:
		return errors.New("Invalid Purpose for File Endpoint")
	}

	return nil
}

// We don't have snapshots on the file so we just say that the whole file is modified.
func (fep *fileEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)
	maxSleepTime := time.Millisecond * 500

	var off metadata.OffsetT

	for off < metadata.OffsetT(fep.lunSize) {
		var chunk *metadata.ChunkInfo

		if shouldAbort() {
			return errors.New("Stopped due to abort condition")
		}

		chunk = &metadata.ChunkInfo{Offset: off, Length: metadata.ObjSize, Dirty: true}
		fep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)

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
		off += metadata.ObjSize
	}
	return nil
}

func (fep *fileEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	var totalBytes int

	for totalBytes = 0; totalBytes < metadata.ObjSize; {
		bytes, err := fep.lun.ReadAt(buf[totalBytes:], int64(offset+metadata.OffsetT(totalBytes)))
		if err != nil {
			if err == io.EOF {
				for b := bytes; b < metadata.ObjSize; b++ {
					buf[b] = 0
				}
				bytes = metadata.ObjSize - bytes
			} else {
				return errors.New("ReadAt: " + offset.String() + " " + err.Error())
			}
		}
		fep.stats.Inc(StatNameSrcReadCount)
		totalBytes += bytes
	}

	fep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)

	return nil
}

func (fep *fileEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	var totalBytes int64
	var nbytes int
	var e error

	if fep.args.DestPreZeroed && hint == metadata.ZeroHash {
		fep.stats.Add(StatNameWriteZeroSkipped, metadata.ObjSize)
	} else {
		for totalBytes = 0; totalBytes < metadata.ObjSize; {
			nbytes, e = fep.lun.WriteAt(buf[totalBytes:], int64(offset+metadata.OffsetT(totalBytes)))
			if e != nil {
				return errors.New("WriteAt " + offset.String() + " " + e.Error())
			}
			totalBytes += int64(nbytes)
			fep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)
		}
	}

	fep.stats.Add(StatNameProgress, metadata.ObjSize)

	return nil
}

func (fep *fileEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	// Shouldn't be needed unless the Differ starts sending clean
	fep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	fep.stats.Add(StatNameProgress, metadata.ObjSize)
	return nil
}

func (fep *fileEndpoint) GetIncrSnapName() string {
	return fep.incrSnapName
}

func (fep *fileEndpoint) GetLunSize() int64 {
	return fep.lunSize
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (fep *fileEndpoint) StoreData(name string, data []byte) error {
	return nil
}

// ResteiveData is a catalog read function
func (fep *fileEndpoint) RetreiveData(name string, data []byte) (int, error) {
	return 0, nil
}

func (fep *fileEndpoint) GetListNext(ctx interface{}) (string, interface{}, error) {
	return "", nil, nil
}

func (fep *fileEndpoint) Done(success bool) error {
	switch fep.purpose {
	case source:
		// fep.baseReader.Close() or close the file?
		// fep.incrReader.Close() or close the file?
	case destination:
		fep.lun.Sync()
		fep.lun.Close()
		// Wait for it to finish
	case manipulation:
		// Nothing to do
	default:
		fmt.Printf("Done on a bad Endpoint.")
	}
	return nil
}
