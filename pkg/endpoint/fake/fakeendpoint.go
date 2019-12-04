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


package fake

import (
	"errors"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"github.com/Nuvoloso/kontroller/pkg/metadata"
	"github.com/Nuvoloso/kontroller/pkg/stats"
	"time"
)

type fakeEndpoint struct {
}

// FailSetup force Setup Failure
var FailSetup bool

// FailDone force Done Failure
var FailDone bool

// FailDiffer force Differ Failure
var FailDiffer bool

// FailFetch force Fetch Failure
var FailFetch bool

// FetchDelay force Fetch Delay
var FetchDelay int

// FillDelay force Fill Delay
var FillDelay int

// FailSkip force Skip Failure
var FailSkip bool

// FailFill force Fill Failure
var FailFill bool

// FailStore force Store Failure
var FailStore bool

// FailRetreive force Retreive Failure
var FailRetreive bool

// FailListNext force GetListNext Failure
var FailListNext bool

// IncrSnap set expected IncrSnap
var IncrSnap = "IncrSnap"

// LunSize set expected LunSize
var LunSize int64 = 1024 * 1024 * 2

// DataToRetrieve is the data returned from RetrieveData
var DataToRetrieve string

// ObjectList is the List of objects to return from GetListNext
var ObjectList []string

// EndpointFactory Function to pretend to be the endpoint factory
func EndpointFactory(args *endpoint.Arg, stats *stats.Stats, workerCount uint) (endpoint.EndPoint, error) {
	if FailSetup {
		return nil, errors.New("Forced Error")
	}
	return &fakeEndpoint{}, nil
}

func (fep *fakeEndpoint) Setup() error {
	return nil
}

func (fep *fakeEndpoint) Differ(dataChan chan *metadata.ChunkInfo, shouldAbort endpoint.AbortCheck) error {
	defer close(dataChan)

	if FailDiffer {
		return errors.New("Forced Error")
	}

	return nil
}

func (fep *fakeEndpoint) FetchAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if FailFetch {
		return errors.New("Forced Error")
	}

	for b := 0; b < metadata.ObjSize; b++ {
		buf[b] = 0
	}

	if FetchDelay != 0 {
		time.Sleep(time.Second * time.Duration(FetchDelay))
		FetchDelay = 0
	}

	return nil
}

func (fep *fakeEndpoint) FillAt(offset metadata.OffsetT, hint string, buf []byte) error {
	if FailFill {
		return errors.New("Forced Error")
	}
	if FillDelay != 0 {
		time.Sleep(time.Second * time.Duration(FillDelay))
		FillDelay = 0
	}

	return nil
}

func (fep *fakeEndpoint) SkipAt(offset metadata.OffsetT, hint string) error {
	if FailSkip {
		return errors.New("Forced Error")
	}

	return nil
}

func (fep *fakeEndpoint) GetIncrSnapName() string {
	return IncrSnap
}

func (fep *fakeEndpoint) GetLunSize() int64 {
	return LunSize
}

func (fep *fakeEndpoint) StoreData(name string, data []byte) error {
	if FailStore {
		return errors.New("Forced Error")
	}
	return nil
}

func (fep *fakeEndpoint) RetreiveData(name string, data []byte) (int, error) {
	if FailRetreive {
		return 0, errors.New("Forced Error")
	}

	expectedData := []byte(DataToRetrieve)
	for i := 0; i < len(DataToRetrieve); i++ {
		data[i] = expectedData[i]
	}
	return len(DataToRetrieve), nil
}

func (fep *fakeEndpoint) GetListNext(token interface{}) (string, interface{}, error) {
	if FailListNext {
		return "", nil, errors.New("Forced Error")
	}
	if len(ObjectList) == 0 {
		return "", nil, nil
	}

	obj := ObjectList[0]
	ObjectList = ObjectList[1:]

	return obj, 1, nil
}

func (fep *fakeEndpoint) Done(success bool) error {
	if FailDone {
		return errors.New("Forced Error")
	}

	return nil
}
