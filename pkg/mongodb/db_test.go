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


package mongodb

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
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

// mockError is a concrete data type used to return error codes
type mockError struct {
	M string
	C int // should be a Mongo error code
}

var _ = error(&mockError{})

// Error matches the error interface
func (e *mockError) Error() string { return e.M }

var errUnknownError = &mockError{M: "unknown error", C: ECUnknownError}

func TestDBAPI(t *testing.T) {
	assert := assert.New(t)
	tracker := &fatalTracker{t: t}
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	url := "mongodb://localhost"
	to := 21 * time.Second
	delay := 10 * time.Millisecond
	dbAPI := NewDBAPI(&Args{URL: url, DatabaseName: dbName, Timeout: to, RetryIncrement: delay, MaxRetryInterval: 2 * delay, Log: l})
	mdb, ok := dbAPI.(*mongoDB)
	assert.True(ok)
	assert.Equal(dbName, dbAPI.DBName())
	assert.Equal(l, dbAPI.Logger())

	t.Log("case: success")
	mockCtrl := gomock.NewController(tracker)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	client := NewMockClient(mockCtrl)
	var fakeCallCount int
	var inFakeConnectOpts []*options.ClientOptions
	var fakeConnectErr error
	d := func(ctx context.Context, opts ...*options.ClientOptions) (Client, error) {
		fakeCallCount++
		assert.NotNil(ctx)
		_, hasDeadline := ctx.Deadline()
		assert.False(hasDeadline)
		inFakeConnectOpts = opts
		if fakeConnectErr != nil {
			return nil, fakeConnectErr
		}
		return client, nil
	}
	mongoConnectHook = d
	dialHookCount := 0
	var dialer *net.Dialer
	var dialerConfig *tls.Config
	var dialerNetwork, dialerAddr string
	tlsDialHook = func(d *net.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error) {
		dialHookCount++
		dialer = d
		dialerConfig = config
		dialerNetwork, dialerAddr = network, addr
		return nil, errors.New("dial hook")
	}
	defer func() {
		mongoConnectHook = connect
		tlsDialHook = tls.DialWithDialer
	}()
	client.EXPECT().Ping(ctx, nil).Return(nil)
	tl.Flush()

	registry := ObjectDocumentHandlerMap{}
	assert.NoError(mdb.Connect(registry))
	assert.Equal(DBReady, mdb.state)
	if tracker.IsFatal() {
		t.FailNow()
	}
	tracker.passthru = true
	assert.Equal(client, mdb.client)
	assert.Equal(client, mdb.Client())
	assert.Equal(to, mdb.DBTimeout())
	if assert.Len(inFakeConnectOpts, 1) {
		assert.Equal(&to, inFakeConnectOpts[0].ConnectTimeout)
		assert.Equal(&to, inFakeConnectOpts[0].ServerSelectionTimeout)
		assert.Equal(&to, inFakeConnectOpts[0].SocketTimeout)
		assert.Nil(inFakeConnectOpts[0].MaxPoolSize)
		assert.Nil(inFakeConnectOpts[0].TLSConfig)
	}
	client.EXPECT().Disconnect(testutils.NewCtxTimeoutMatcher(to)).Return(nil)
	mdb.Terminate()
	assert.Equal(DBNotConnected, mdb.state)
	assert.Nil(mdb.client)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: retry for connect and ping failures, cover MaxPoolSize, UseSSL success")
	tracker.passthru = false
	mdb.UseSSL = true
	mdb.MaxPoolSize = 2112
	mdb.TLSCACertificate = "./test-data/ca.crt"
	mdb.SSLServerName = "set"
	mockCtrl = gomock.NewController(tracker)
	client = NewMockClient(mockCtrl)
	prev := client.EXPECT().Ping(ctx, nil).Return(errUnknownError)
	client.EXPECT().Ping(ctx, nil).Return(nil).After(prev)
	fakeCallCount = 0
	fakeConnectErr = errUnknownError
	assert.NoError(mdb.Connect(registry))
	assert.NotEqual(mdb.state, DBReady)
	assert.NotZero(fakeCallCount)
	fakeConnectErr = nil
	for mdb.state != DBReady && !tracker.IsFatal() {
		time.Sleep(20 * time.Millisecond)
	}
	if tracker.IsFatal() {
		t.FailNow()
	}
	tracker.passthru = true
	assert.Equal(DBReady, mdb.state)
	assert.NotNil(mdb.client)
	if assert.Len(inFakeConnectOpts, 1) {
		assert.Equal(&to, inFakeConnectOpts[0].ConnectTimeout)
		assert.Equal(&to, inFakeConnectOpts[0].SocketTimeout)
		assert.EqualValues(&mdb.MaxPoolSize, inFakeConnectOpts[0].MaxPoolSize)
		assert.Nil(inFakeConnectOpts[0].TLSConfig)
		assert.Equal(mdb, inFakeConnectOpts[0].Dialer)
		assert.False(swag.BoolValue(inFakeConnectOpts[0].Direct))
		if assert.NotNil(mdb.cfg) {
			cfg := mdb.cfg
			assert.Empty(cfg.Certificates)
			assert.NotNil(cfg.RootCAs)
			assert.Equal("set", cfg.ServerName)
			assert.False(cfg.InsecureSkipVerify)
		}
	}
	assert.Zero(dialHookCount)
	client.EXPECT().Disconnect(testutils.NewCtxTimeoutMatcher(to)).Return(nil)
	mdb.Terminate()
	assert.Nil(mdb.client)
	tl.Flush()
	mdb.MaxPoolSize = 0
	mdb.UseSSL = false
	mockCtrl.Finish()

	t.Log("case: call DialContext no deadline")
	conn, err := mdb.DialContext(context.Background(), "tcp", "localhost")
	assert.Nil(conn)
	assert.Regexp("dial hook", err)
	assert.Equal(1, dialHookCount)
	assert.Equal(&net.Dialer{Timeout: to}, dialer)
	assert.NotNil(mdb.cfg)
	assert.Equal(mdb.cfg, dialerConfig)
	assert.Equal("tcp", dialerNetwork)
	assert.Equal("localhost", dialerAddr)

	t.Log("case: call DialContext with deadline")
	dl := time.Now().Add(to)
	ctx, cancel := context.WithDeadline(context.Background(), dl)
	conn, err = mdb.DialContext(ctx, "tcp", "localhost")
	cancel()
	assert.Nil(conn)
	assert.Regexp("dial hook", err)
	assert.Equal(2, dialHookCount)
	assert.Equal(&net.Dialer{Timeout: to, Deadline: dl}, dialer)
	assert.NotNil(mdb.cfg)
	assert.Equal(mdb.cfg, dialerConfig)
	assert.Equal("tcp", dialerNetwork)
	assert.Equal("localhost", dialerAddr)

	t.Log("case: failure in odh.Initialize() and odh.Start(), each recovers on 2nd attempt, cover DirectConnect")
	tracker.passthru = false
	mdb.DirectConnect = true
	mockCtrl = gomock.NewController(tracker)
	client = NewMockClient(mockCtrl)
	ctx = context.Background()
	client.EXPECT().Ping(ctx, readpref.Nearest()).Return(nil).MinTimes(1)
	client.EXPECT().Disconnect(testutils.NewCtxTimeoutMatcher(to)).Return(nil).Times(2)
	odh := NewMockObjectDocumentHandler(mockCtrl)
	prev = odh.EXPECT().Initialize(ctx).Return(errors.New("init error"))
	odh.EXPECT().Initialize(ctx).Return(nil).Times(2).After(prev)
	prev = odh.EXPECT().Start(ctx).Return(errors.New("start error"))
	odh.EXPECT().Start(ctx).Return(nil).After(prev)
	registry["mock"] = odh
	assert.NoError(mdb.Connect(registry))
	assert.NotEqual(mdb.state, DBReady)
	for mdb.state != DBReady && !tracker.IsFatal() {
		time.Sleep(20 * time.Millisecond)
		tl.Flush()
	}
	if tracker.IsFatal() {
		t.FailNow()
	}
	tracker.passthru = true
	assert.Equal(DBReady, mdb.state)
	if assert.Len(inFakeConnectOpts, 1) {
		assert.True(swag.BoolValue(inFakeConnectOpts[0].Direct))
	}
	client.EXPECT().Disconnect(testutils.NewCtxTimeoutMatcher(to)).Return(nil)
	mdb.Terminate()
	mdb.DirectConnect = false
	tl.Flush()

	t.Log("case: Disconnect fails, fatal error")
	tracker.passthru = false
	exitHook = func(int) { panic("exitHook called") }
	mdb.client = client
	mdb.state = DBReady
	client.EXPECT().Disconnect(testutils.NewCtxTimeoutMatcher(to)).Return(errUnknownError)
	assert.PanicsWithValue("exitHook called", func() { mdb.Terminate() })
	tracker.passthru = true
	tl.Flush()

	t.Log("case: Start fails due to invalid URL")
	mdb.URL = "localhost"
	assert.Regexp("error parsing uri", mdb.Connect(registry))

	t.Log("case: Start fails due to bad SSL config")
	mdb.TLSCertificate = "no such file"
	mdb.UseSSL = true
	assert.Regexp("no such file", mdb.Connect(registry))
	mdb.UseSSL = false
}

func TestMustBeInitializing(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dbAPI := NewDBAPI(&Args{Log: l})
	mdb, ok := dbAPI.(*mongoDB)
	assert.True(ok)
	mdb.state = DBInitializing
	assert.NoError(mdb.MustBeInitializing())
	mdb.state = DBReady
	assert.Regexp("not initializing", mdb.MustBeInitializing())
}

func TestMustBeReady(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dbAPI := NewDBAPI(&Args{Log: l})
	mdb, ok := dbAPI.(*mongoDB)
	assert.True(ok)
	mdb.state = DBInitializing
	assert.Regexp("not available", mdb.MustBeReady())
	mdb.state = DBReady
	assert.NoError(mdb.MustBeReady())
}

func TestErrorCode(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	delay := time.Second
	dbAPI := NewDBAPI(&Args{URL: "localhost", DatabaseName: dbName, Timeout: 25 * time.Second, RetryIncrement: delay, MaxRetryInterval: delay, Log: l})
	assert.NotNil(dbAPI)
	mdb, ok := dbAPI.(*mongoDB)
	assert.True(ok)
	assert.Equal(DBNotConnected, mdb.state)

	// nil error, normal case leaves mdb.state unchanged
	assert.Equal(mdb.ErrorCode(nil), ECKeyNotFound)
	assert.Equal(DBNotConnected, mdb.state)

	// nil error while state is DBError, state becomes DBReady
	mdb.state = DBError
	assert.Equal(ECKeyNotFound, mdb.ErrorCode(nil))
	assert.Equal(DBReady, mdb.state)

	// mongo.CommandError, mongo.WriteException and ErrNoDocuments return the error Code, state remains DBReady
	assert.Equal(43, mdb.ErrorCode(mongo.CommandError{Code: 43}))
	assert.Equal(DBReady, mdb.state)
	assert.Equal(ECUnknownError, mdb.ErrorCode(mongo.WriteException{}))
	assert.Equal(DBReady, mdb.state)
	assert.Equal(42, mdb.ErrorCode(mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{Code: 42}}))
	assert.Equal(DBReady, mdb.state)
	wes := []mongo.WriteError{{Code: 21}, {Code: 11000}}
	assert.Equal(42, mdb.ErrorCode(mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{Code: 42}, WriteErrors: wes}))
	assert.Equal(DBReady, mdb.state)
	assert.Equal(21, mdb.ErrorCode(mongo.WriteException{WriteErrors: wes}))
	assert.Equal(DBReady, mdb.state)
	assert.Equal(ECDuplicateKey, mdb.ErrorCode(mongo.WriteException{WriteErrors: wes[1:]}))
	assert.Equal(DBReady, mdb.state)
	assert.Equal(ECKeyNotFound, mdb.ErrorCode(mongo.ErrNoDocuments))
	assert.Equal(DBReady, mdb.state)

	// mongo.CommandError, mongo.WriteException and ErrNoDocuments return the error Code, state becomes DBReady
	mdb.state = DBError
	assert.Equal(43, mdb.ErrorCode(mongo.CommandError{Code: 43}))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(ECUnknownError, mdb.ErrorCode(mongo.WriteException{}))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(42, mdb.ErrorCode(mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{Code: 42}}))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(42, mdb.ErrorCode(mongo.WriteException{WriteConcernError: &mongo.WriteConcernError{Code: 42}, WriteErrors: wes}))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(21, mdb.ErrorCode(mongo.WriteException{WriteErrors: wes}))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(ECDuplicateKey, mdb.ErrorCode(mongo.WriteException{WriteErrors: wes[1:]}))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(ECKeyNotFound, mdb.ErrorCode(mongo.ErrNoDocuments))
	assert.Equal(DBReady, mdb.state)

	// errors that cause transition to DBError
	assert.Equal(ECSocketException, mdb.ErrorCode(&net.OpError{Op: "op", Err: errors.New("op error")}))
	assert.Equal(DBError, mdb.state)
	mdb.state = DBReady
	assert.Equal(ECInterrupted, mdb.ErrorCode(io.EOF))
	assert.Equal(DBError, mdb.state)
	mdb.state = DBReady
	assert.Equal(ECInterrupted, mdb.ErrorCode(context.DeadlineExceeded))
	assert.Equal(DBError, mdb.state)
	mdb.state = DBReady
	assert.Equal(ECInterrupted, mdb.ErrorCode(mongo.ErrUnacknowledgedWrite))
	assert.Equal(DBError, mdb.state)
	mdb.state = DBReady
	assert.Equal(ECInterrupted, mdb.ErrorCode(mongo.ErrClientDisconnected))
	assert.Equal(DBError, mdb.state)
	mdb.state = DBReady
	assert.Equal(ECHostUnreachable, mdb.ErrorCode(errors.New("server selection error: blah blah blah")))
	assert.Equal(DBError, mdb.state)
	mdb.state = DBReady

	// unknown error, mdb state remains unchanged
	assert.Equal(ECUnknownError, mdb.ErrorCode(errors.New("an error")))
	assert.Equal(DBReady, mdb.state)
	mdb.state = DBError
	assert.Equal(ECUnknownError, mdb.ErrorCode(errors.New("another error")))
	assert.Equal(DBError, mdb.state)
}

func TestMongoInterface(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	dbName := "testDB"
	cName := "testCollection"

	// need to call real connect function here to get a valid client. connect does not actually open a socket
	t.Log("case: call actual connect func, failure")
	client, err := connect(context.Background(), options.Client().ApplyURI("invalid"))
	assert.Regexp("error parsing uri", err)
	assert.Nil(client)

	t.Log("case: call actual connect func, success")
	client, err = connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:17"))
	assert.NoError(err)
	assert.NotNil(client)
	mcl, ok := client.(*MongoClient)
	assert.True(ok)
	assert.NotNil(mcl.Client)

	// without an actual DB connection, can only cover failures for several of the functions
	t.Log("case: Ping() timeout")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancel()
	assert.Error(client.Ping(ctx, nil)) // actual error may not be context.DeadlineExceeded, OS-dependant

	db := client.Database(dbName)
	assert.NotNil(db)
	md, ok := db.(*MongoDatabase)
	assert.True(ok)
	assert.NotNil(md.Database)
	assert.Equal(dbName, md.Database.Name())

	assert.Error(db.RunCommand(ctx, bson.M{"ping": 1}).Err())

	c := db.Collection(cName)
	assert.NotNil(c)
	mc, ok := c.(*MongoCollection)
	assert.True(ok)
	assert.NotNil(mc.Collection)
	assert.Equal(cName, mc.Collection.Name())

	iv := c.Indexes()
	assert.NotNil(iv)
	_, ok = iv.(*MongoIndexView)
	assert.True(ok)

	cur, err := c.Aggregate(ctx, []bson.D{{}, {}})
	assert.Nil(cur)
	assert.Error(err)

	cur, err = c.Find(ctx, bson.M{})
	assert.Nil(cur)
	assert.Error(err)

	sr := c.FindOne(ctx, bson.M{})
	assert.NotNil(sr)
	_, ok = sr.(*MongoSingleResult)
	assert.True(ok)

	sr = c.FindOneAndUpdate(ctx, bson.M{}, bson.M{})
	assert.NotNil(sr)
	_, ok = sr.(*MongoSingleResult)
	assert.True(ok)

	err = client.Disconnect(ctx)
	assert.NoError(err) // unsure why this succeeds using ctx which is timed out. it hangs with context.Background()
}
