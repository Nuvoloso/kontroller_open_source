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
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"time"
)

// FakeArgs is the set of arguments expected for a fake endpoint
type FakeArgs struct {
	FailConfig bool
	FailDiffer bool
	FailFetch  bool
	FailFill   bool
	FailSkip   bool
	FailDone   bool
	FetchDelay int
	FillDelay  int
}

type fakeEndpoint struct {
	purpose Purpose
	args    *FakeArgs
	stats   *stats.Stats
}

func (fep *fakeEndpoint) Setup() error {
	if fep.args.FailConfig {
		return errors.New("Forced Error")
	}

	return nil
}

func (fep *fakeEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort AbortCheck) error {
	defer close(dataChan)

	if fep.args.FailDiffer {
		return errors.New("Forced Error")
	}

	dataChan <- &metadata.ChunkInfo{
		Offset: 0,
		Length: metadata.ObjSize,
		Hash:   "hash",
		Dirty:  false,
	}
	fep.stats.Add(StatNameDiffBytesUnchanged, metadata.ObjSize)

	dataChan <- &metadata.ChunkInfo{
		Offset: metadata.ObjSize,
		Length: metadata.ObjSize,
		Hash:   "hash",
		Dirty:  true,
	}
	fep.stats.Add(StatNameDiffBytesChanged, metadata.ObjSize)

	dataChan <- &metadata.ChunkInfo{}

	return nil
}

func (fep *fakeEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if fep.args.FailFetch {
		return errors.New("Forced Error")
	}

	for b := 0; b < metadata.ObjSize; b++ {
		buf[b] = 0
	}
	fep.stats.Add(StatNameSrcBytesRead, metadata.ObjSize)
	fep.stats.Inc(StatNameSrcReadCount)

	if fep.args.FetchDelay != 0 {
		time.Sleep(time.Second * time.Duration(fep.args.FetchDelay))
		fep.args.FetchDelay = 0
	}

	return nil
}

func (fep *fakeEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if fep.args.FailFill {
		return errors.New("Forced Error")
	}
	if fep.args.FillDelay != 0 {
		time.Sleep(time.Second * time.Duration(fep.args.FillDelay))
		fep.args.FillDelay = 0
	}
	fep.stats.Add(StatNameDestBytesWritten, metadata.ObjSize)
	fep.stats.Add(StatNameProgress, metadata.ObjSize)

	return nil
}

func (fep *fakeEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	if fep.args.FailSkip {
		return errors.New("Forced Error")
	}
	fep.stats.Add(StatNameDestBytesSkipped, metadata.ObjSize)
	fep.stats.Add(StatNameProgress, metadata.ObjSize)

	return nil
}

func (fep *fakeEndpoint) GetIncrSnapName() string {
	return "IncrSnap"
}

func (fep *fakeEndpoint) GetLunSize() int64 {
	return 2 * metadata.ObjSize
}

// StoreData is a catalog writing function. Data will be stored in the
// endpoint according the the domain, and encryption policy of the endpoint.
func (fep *fakeEndpoint) StoreData(name string, data []byte) error {
	return nil
}

// RestriveData catalog read function
func (fep *fakeEndpoint) RetreiveData(name string, data []byte) (int, error) {
	return 0, nil
}

func (fep *fakeEndpoint) GetListNext(ctx interface{}) (string, interface{}, error) {
	return "", nil, nil
}

func (fep *fakeEndpoint) Done(success bool) error {
	if fep.args.FailDone {
		return errors.New("Forced Error")
	}

	return nil
}
