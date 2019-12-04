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


package mongods

import (
	"runtime"
	"testing"
	"time"

	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

// fatalTracker implements gomock.TestReporter to catch fatal errors while testing the ds.start() goroutine
type fatalTracker struct {
	t        *testing.T
	passthru bool
	fatal    bool
}

// Errorf always passes thru to the underlying testing.T.Errorf()
func (f *fatalTracker) Errorf(format string, args ...interface{}) {
	f.t.Errorf(format, args...)
}

// Fatalf sets the fatal flag
// if passthru is false (the default), the error is noted and the goroutine exits. The main goroutine must check f.IsFatal().
// if passthru is true, it calls the underlying testing.T.Fatalf(). This should only occur in the main test goroutine.
func (f *fatalTracker) Fatalf(format string, args ...interface{}) {
	f.fatal = true
	if f.passthru {
		f.t.Fatalf(format, args...)
	} else {
		// do not call f.t.Fatalf() here, it marks the test as finished that should not happen in the goroutine
		f.t.Errorf(format, args...)
		runtime.Goexit()
	}
}

// IsFatal returns true of the fatalTracker has detected a fatal test failure
func (f *fatalTracker) IsFatal() bool {
	return f.fatal
}

func TestDSStartStop(t *testing.T) {
	assert := assert.New(t)
	tracker := &fatalTracker{t: t}
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testPath := "" // disables datastore initialize logic in role and service plan ODH
	url := "mongodb://localhost"
	to := 21 * time.Second
	delay := 10 * time.Millisecond
	mds := NewDataStore(&MongoDataStoreArgs{Args: mongodb.Args{URL: url, DatabaseName: dbName, Timeout: to, RetryIncrement: delay, MaxRetryInterval: 2 * delay, Log: l}, BaseDataPath: testPath})
	assert.NotNil(mds.DBAPI)
	assert.NotNil(mds.dsCRUD)
	assert.Equal(mds.BaseDataPath, mds.BaseDataPathName())

	t.Log("case: success")
	mockCtrl := gomock.NewController(tracker)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	api.EXPECT().Connect(odhRegistry).Return(nil)
	mds.DBAPI = api
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	mds.dsCRUD = crud
	assert.NoError(mds.Start())
	assert.NotNil(mds.OpsAccount())
	assert.NotNil(mds.OpsApplicationGroup())
	assert.NotNil(mds.OpsCluster())
	assert.NotNil(mds.OpsConsistencyGroup())
	assert.NotNil(mds.OpsCspCredential())
	assert.NotNil(mds.OpsCspDomain())
	assert.NotNil(mds.OpsNode())
	assert.NotNil(mds.OpsPool())
	assert.NotNil(mds.OpsProtectionDomain())
	assert.NotNil(mds.OpsRole())
	assert.NotNil(mds.OpsServicePlan())
	assert.NotNil(mds.OpsServicePlanAllocation())
	assert.NotNil(mds.OpsSnapshot())
	assert.NotNil(mds.OpsStorage())
	assert.NotNil(mds.OpsStorageRequest())
	assert.NotNil(mds.OpsSystem())
	assert.NotNil(mds.OpsUser())
	assert.NotNil(mds.OpsVolumeSeries())
	assert.NotNil(mds.OpsVolumeSeriesRequest())
	if tracker.IsFatal() {
		t.FailNow()
	}
	tl.Flush()

	api.EXPECT().Terminate()
	mds.Stop()

	t.Log("case: failure (cover unexpected odh)")
	odhRegister("fake", &fakeODH{})
	mds = NewDataStore(&MongoDataStoreArgs{Args: mongodb.Args{URL: url, DatabaseName: dbName, Timeout: to, RetryIncrement: delay, MaxRetryInterval: 2 * delay, Log: l}, BaseDataPath: testPath})
	api.EXPECT().Connect(odhRegistry).Return(errUnknownError)
	mds.DBAPI = api
	mds.dsCRUD = crud
	assert.Equal(errUnknownError, mds.Start())
	delete(odhRegistry, "fake")
}

func TestWrapError(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	delay := time.Second
	mds := NewDataStore(&MongoDataStoreArgs{Args: mongodb.Args{URL: "mongodb://localhost", DatabaseName: dbName, Timeout: 25 * time.Second, RetryIncrement: delay, MaxRetryInterval: delay, Log: l}})
	assert.NotNil(mds)

	// nil error not converted
	assert.Nil(mds.WrapError(nil, false))
	assert.Nil(mds.WrapError(nil, true))

	// NotFound special case handling varies between insert and update
	assert.Equal(ds.ErrorNotFound, mds.WrapError(mongo.ErrNoDocuments, false))
	assert.Equal(ds.ErrorIDVerNotFound, mds.WrapError(mongo.ErrNoDocuments, true))

	// Duplicate Key special case
	assert.Equal(ds.ErrorExists, mds.WrapError(mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000}}}, false))
	assert.Equal(ds.ErrorExists, mds.WrapError(mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 11000}}}, true))

	// default
	mErr := mongo.CommandError{Code: 86, Message: "IndexKeySpecsConflict"}
	err := mds.WrapError(mErr, false)
	assert.Error(err)
	assert.Equal(err, mds.WrapError(mErr, true))
	if dsError, ok := err.(*ds.Error); assert.True(ok, "centrald.Error expected") {
		assert.Equal(ds.ErrorDbError.C, dsError.C)
	}
	assert.Regexp(ds.ErrorDbError.M+".*"+mErr.Message+".*86.$", err)
}
