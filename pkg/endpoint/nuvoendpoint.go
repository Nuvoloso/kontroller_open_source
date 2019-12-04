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
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"io"
	"os"
	"time"
)

const maxReadSize = 32 * 1024
const maxAPITries = 7

// NuvoArgs is the set of arguments expected for an Nuvo copy endpoint
type NuvoArgs struct {
	VolumeUUID    string
	FileName      string
	BaseSnapUUID  string
	IncrSnapUUID  string
	NuvoSocket    string
	DestPreZeroed bool
}

type nuvoEndpoint struct {
	args         *NuvoArgs
	purpose      Purpose
	volUUID      string
	baseSnapUUID string
	incrSnapUUID string
	lunSize      metadata.OffsetT
	stats        *stats.Stats
	lun          *os.File
	NuvoSocket   string
}

func (nep *nuvoEndpoint) Setup() error {
	var err error

	args := nep.args

	nep.volUUID = args.VolumeUUID
	nep.baseSnapUUID = args.BaseSnapUUID
	nep.incrSnapUUID = args.IncrSnapUUID
	nep.NuvoSocket = args.NuvoSocket

	fi, e := os.Stat(args.FileName)
	if e != nil {
		return errors.New("Stat: " + e.Error())
	}
	nep.lunSize = metadata.OffsetT(fi.Size())

	switch nep.purpose {
	case destination:
		nep.lun, err = os.OpenFile(args.FileName, os.O_WRONLY, 0755)
		if err != nil {
			return errors.New(nep.purpose.String() + " cannot open nuvo LUN")
		}
	case source:
		nep.lun, err = os.Open(args.FileName)
		if err != nil {
			return errors.New(nep.purpose.String() + " cannot open nuvo LUN")
		}
		nep.lun.Sync()
		nep.stats.Set(StatNameSrcReadCount, 0)

	case manipulation:
		// Nothing to do
	default:
		return errors.New("Invalid Purpose for Dir Endpoint")
	}

	fmt.Printf("%s Nuvo Endpoint Vol %s, size %dM\n", nep.purpose.String(), nep.volUUID, nep.lunSize)

	return nil
}

// Get a bunch of diffs from storelandia
func (nep *nuvoEndpoint) getNextDiffs(off metadata.OffsetT) ([]*metadata.ChunkInfo, metadata.OffsetT, error) {
	var diffBlock []*metadata.ChunkInfo

	if off > nep.lunSize {
		return nil, nep.lunSize, errors.New("offset beyond lun size")
	}

	if off == nep.lunSize {
		return nil, off, nil
	}

	var trys int
	var progress uint64
	var diffs []nuvoapi.Diff
	var err error

	for ; trys < maxAPITries; trys++ {
		nuvoAPI := nuvoapi.NewNuvoVM(nep.NuvoSocket)
		diffs, progress, err = nuvoAPI.GetPitDiffs(nep.volUUID, nep.baseSnapUUID, nep.incrSnapUUID, uint64(off))
		if err == nil {
			break
		}

		if !nuvoapi.ErrorIsTemporary(err) {
			return nil, off, errors.New("getNextDiffs: " + err.Error())
		}
		if trys == maxAPITries {
			return nil, off, errors.New("getNextDiffs: too many retries")
		}

		time.Sleep((2*time.Duration(trys) + 1) * time.Second)
	}

	for i := 0; i < len(diffs); i++ {
		diff := &metadata.ChunkInfo{
			Offset: metadata.OffsetT(diffs[i].Start),
			Length: metadata.OffsetT(diffs[i].Length),
			Dirty:  diffs[i].Dirty}

		diffBlock = append(diffBlock, diff)

		off += metadata.OffsetT(metadata.ObjSize)
	}

	// if returned offset > lastdiff.Offset + lastdiff.Length {
	//  put a Clean diff on the end, done at the level above
	// }

	return diffBlock, metadata.OffsetT(progress), nil
}

func (nep *nuvoEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)
	maxSleepTime := time.Millisecond * 500

	// while we haven't hit the end of the volume
	for off := metadata.OffsetT(0); off < nep.lunSize; {
		diffs, progress, err := nep.getNextDiffs(off)
		if err != nil {
			return errors.New("getting diffs failed: " + err.Error())
		}

		// while we have some diffs to process
		for i := 0; i < len(diffs); {
			diff := diffs[i]
			if diff.Offset == off {
				i++ // only increment if we consume one
			} else {
				// Create an entry that says not dirty
				// for the whole gap.  The next loop
				// will break it down to sizeable chunks.
				diff = &metadata.ChunkInfo{
					Offset: off,
					Length: diff.Offset - off,
					Dirty:  false,
				}
				nep.stats.Inc(StatNameDiffGaps)
			}

			if diff.Offset%metadata.ObjSize != 0 ||
				diff.Length%metadata.ObjSize != 0 {
				return errors.New("misaligned diff ")
			}

			if diff.Dirty {
				nep.stats.Add(StatNameDiffBytesChanged, int64(diff.Length))
			} else {
				nep.stats.Add(StatNameDiffBytesUnchanged, int64(diff.Length))
			}

			// Produce all properly sized diffs to be processed
			for consumed := metadata.OffsetT(0); consumed < diff.Length; consumed += metadata.ObjSize {
				if shouldAbort() {
					return errors.New("Stopped due to abort condition")
				}

				chunk := &metadata.ChunkInfo{
					Offset: diff.Offset + consumed,
					Length: metadata.ObjSize,
					Dirty:  diff.Dirty}

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
			off += diff.Length
		}
		off = progress
	}
	return nil
}

// min is just a quick minimum for ints
func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

func (nep *nuvoEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	// maxReadSize is used here to reduce the size of the reads to
	// limit the impact of backup I/O against user activity
	// We could bump this back up to metadata.ObjSize

	var totalBytes int

	for totalBytes = 0; totalBytes < metadata.ObjSize; {
		readEnd := min(totalBytes+maxReadSize, metadata.ObjSize)

		bytes, err := nep.lun.ReadAt(buf[totalBytes:readEnd], int64(offset+metadata.OffsetT(totalBytes)))
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
		nep.stats.Inc(StatNameSrcReadCount)
		totalBytes += bytes
	}

	nep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)

	return nil
}

func (nep *nuvoEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	var totalBytes int64
	var nbytes int
	var e error

	if nep.args.DestPreZeroed && hint == metadata.ZeroHash {
		nep.stats.Add(StatNameWriteZeroSkipped, metadata.ObjSize)
	} else {
		for totalBytes = 0; totalBytes < metadata.ObjSize; {
			nbytes, e = nep.lun.WriteAt(buf[totalBytes:], int64(offset+metadata.OffsetT(totalBytes)))
			if e != nil {
				return errors.New("WriteAt " + offset.String() + " " + e.Error())
			}
			totalBytes += int64(nbytes)
		}
		nep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)
	}

	nep.stats.Add(StatNameProgress, metadata.ObjSize)

	return nil
}

func (nep *nuvoEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	nep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	nep.stats.Add(StatNameProgress, metadata.ObjSize)
	return nil
}

func (nep *nuvoEndpoint) GetIncrSnapName() string {
	return nep.incrSnapUUID
}

func (nep *nuvoEndpoint) GetLunSize() int64 {
	// This should return a uint64 instead
	return int64(nep.lunSize)
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (nep *nuvoEndpoint) StoreData(name string, data []byte) error {
	return errors.New("StoreData for a nuvo endpoint not supported")
}

// ResteiveData is a catalog read function
func (nep *nuvoEndpoint) RetreiveData(name string, data []byte) (int, error) {
	return 0, errors.New("RetrieveData for a nuvo endpoint not supported")
}

func (nep *nuvoEndpoint) GetListNext(ctx interface{}) (string, interface{}, error) {
	return "", nil, nil
}

func (nep *nuvoEndpoint) Done(success bool) error {
	switch nep.purpose {
	case source:
		nep.lun.Close()
		// nep.baseReader.Close() or close the file?
		// nep.incrReader.Close() or close the file?
	case destination:
		// Close the mdChan to stop the metadata handler
		// Wait for it to finish
		nep.lun.Sync()
		nep.lun.Close()
		// Should a snapshot be created on the destination
		// to match what is done on other endpoints?
		// This might be too disruptive from copy.
	case manipulation:
		// Nothing to do
	default:
		fmt.Printf("Done on a bad Endpoint.")
	}

	return nil
}
