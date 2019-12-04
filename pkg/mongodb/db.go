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


// Package mongodb abstracts the mongodb client interface
package mongodb

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-openapi/runtime/client"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Args contains the arguments to create a DBAPI
type Args struct {
	URL              string        `long:"url" description:"Mongo server URL" default:"mongodb://localhost"`
	DatabaseName     string        `long:"db" description:"Name of database" default:"nuvodb"`
	MaxPoolSize      uint64        `long:"max-pool-size" description:"Maximum size of a server's connection pool. If zero, the driver default is used" default:"0"`
	Timeout          time.Duration `long:"timeout" description:"Timeout for a database operation" default:"25s"`
	RetryIncrement   time.Duration `long:"dial-retry-increment" hidden:"true" default:"10s"`
	MaxRetryInterval time.Duration `long:"max-retry-interval" hidden:"true" default:"60s"`
	UseSSL           bool          `long:"ssl" description:"Use SSL to communicate with the datastore"`
	SSLServerName    string        `long:"ssl-server-name" description:"The actual server name of the datastore SSL certificate"`
	DirectConnect    bool          `no-flag:"1"` // set on startup in a client that can tolerate replica set issues, such as before rs.initiate() is performed

	// expected to be copied from service on startup
	Log               *logging.Logger `json:"-"`
	AppName           string          `no-flag:"1"`
	TLSCertificate    string          `no-flag:"1"`
	TLSCertificateKey string          `no-flag:"1"`
	TLSCACertificate  string          `no-flag:"1"`
}

// NewDBAPI returns a DBAPI
func NewDBAPI(args *Args) DBAPI {
	ds := &mongoDB{
		Args: *args,
	}
	return ds
}

// DBState is the state of the mongoDB
type DBState int

// mongoDB states
// The state is initially DBNotConnected and remains there until Dial() succeeds, transitioning to DBInitializing.
// If any errors occur while in DBInitializing, the session is closed and the state returns to DBNotConnected.
// After initialization succeeds the state transitions to DBReady.
// It remains in this state unless some sort of connection error occurs.
// In that case, it transitions to DBError until the session is used again successfully, at
// which time it transitions back to DBReady.
// When Terminate() is called, the state transitions back to DBNotConnected.
const (
	DBNotConnected DBState = iota
	DBInitializing
	DBReady
	DBError
)

// mongoDB contains the datastore connection information
// It satisfies the following interfaces:
// - DBAPI
type mongoDB struct {
	Args
	cfg           *tls.Config
	clientOptions *options.ClientOptions
	client        Client
	state         DBState
	mux           sync.Mutex
}

var _ = DBAPI(&mongoDB{})

type connectFn func(ctx context.Context, opts ...*options.ClientOptions) (Client, error)

// mongoConnectHook exists to aid UT
var mongoConnectHook connectFn = connect

func connect(ctx context.Context, opts ...*options.ClientOptions) (Client, error) {
	client, err := mongo.Connect(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &MongoClient{client}, nil
}

// start is run on a goroutine and performs all one-time work as soon as the Mongo database is available.
// It uses the provided channel to allow the Connect() function to wait until the first attempt is complete.
func (db *mongoDB) start(registry ObjectDocumentHandlerMap, tried chan bool) {
	delay := 0 * time.Second
	firstAttempt := true
	// always signal the channel even if this goroutine fails, to let main goroutine continue
	defer func() {
		if firstAttempt {
			tried <- true
		}
	}()
	db.state = DBNotConnected
	db.Log.Info("MongoDB state ⇒ NotConnected")
	for {
		// Connect to mongo
		db.Log.Infof("Connecting to mongo at [%s]", db.URL)
		var err error
		// Not context.WithTimeout(): timeouts are specified in the clientOptions and a context Timeout masks actual error returned by mongo.
		// Empirically, Connect() does not actually open a socket, or if it does, it does not wait for it.
		// Ping() is called below to verify the connection.
		ctx := context.Background()
		var pref *readpref.ReadPref
		if db.DirectConnect {
			// in DirectConnect mode, want Ping to work even if no PRIMARY
			pref = readpref.Nearest()
		}
		if db.client, err = mongoConnectHook(ctx, db.clientOptions); err != nil {
			db.Log.Errorf("Cannot connect to mongo at [%s]: %s", db.URL, err)
			goto Retry
		}
		if err = db.client.Ping(ctx, pref); err != nil {
			db.Log.Errorf("Cannot ping mongo at [%s]: %s", db.URL, err)
			goto Retry
		}
		db.Log.Infof("Connected to mongo at [%s]", db.URL)
		db.state = DBInitializing
		db.Log.Info("MongoDB state ⇒ Initializing")

		// initialize available object handlers
		for name, odh := range registry {
			db.Log.Info("Initializing ODH", name)
			if err := odh.Initialize(ctx); err != nil {
				db.Log.Errorf("Failed to initialize ODH [%s]: %s", name, err.Error())
				goto Retry
			}
		}
		// Start
		for name, odh := range registry {
			db.Log.Info("Starting ODH", name)
			if err := odh.Start(ctx); err != nil {
				db.Log.Errorf("Failed to start ODH [%s]: %s", name, err.Error())
				goto Retry
			}
		}
		break
	Retry:
		db.Terminate()
		if firstAttempt {
			tried <- true
			firstAttempt = false
		}
		if delay < db.MaxRetryInterval {
			delay += db.RetryIncrement
		}
		db.Log.Infof("Will retry MongoDB connection in %d seconds", delay/time.Second)
		time.Sleep(delay)
	}
	db.becomeReady(true)
	db.Log.Info("MongoDB initialization is complete")
	// TBD additional housekeeping
}

// becomeReady will transition the db state to DBReady
// It should be called when a response or error indicates communication with mongo actually occurred
func (db *mongoDB) becomeReady(force bool) {
	if db.state == DBError || force {
		db.mux.Lock()
		defer db.mux.Unlock()
		if db.state == DBError || force {
			db.state = DBReady
			db.Log.Info("MongoDB state ⇒ Ready")
		}
	}
}

type tlsDialFn func(dialer *net.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error)

var tlsDialHook tlsDialFn = tls.DialWithDialer

// DialContext implements the mongo.options.ContextDialer interface. It is set when SSL is enabled.
// Use this rather than the built-in mongo client TLS logic because it overwrites our specified ServerName, requiring the use of InsecureSkipVerify
// Unfortunately, go 1.11.x does not have a tls.DialContext()
func (db *mongoDB) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// TBD provide more complex dialer to handle case when ctx is canceled, not just its deadline. Currently we have no code that attempts to cancel
	tlsDialer := &net.Dialer{Timeout: db.Timeout}
	if dl, ok := ctx.Deadline(); ok {
		tlsDialer.Deadline = dl
	}
	return tlsDialHook(tlsDialer, network, address, db.cfg)
}

// Connect uses a goroutine to open a connection to the Mongo database.
// It blocks until the first connection attempt is complete.
// Errors are only returned when pre-connection initialization fails.
func (db *mongoDB) Connect(registry ObjectDocumentHandlerMap) error {
	clientOptions := options.Client().ApplyURI(db.URL).SetAppName(db.AppName).SetConnectTimeout(db.Timeout).SetServerSelectionTimeout(db.Timeout).SetSocketTimeout(db.Timeout)
	if db.MaxPoolSize > 0 {
		clientOptions.SetMaxPoolSize(db.MaxPoolSize)
	}
	if db.UseSSL {
		tlsClientOpts := client.TLSClientOptions{
			Certificate: db.TLSCertificate,
			Key:         db.TLSCertificateKey,
			CA:          db.TLSCACertificate,
			ServerName:  db.SSLServerName,
		}
		cfg, err := client.TLSClientAuth(tlsClientOpts)
		if err != nil {
			return err
		}
		db.cfg = cfg
		clientOptions.SetDialer(db)
	}
	if db.DirectConnect {
		clientOptions.SetDirect(true)
	}
	if err := clientOptions.Validate(); err != nil {
		return err
	}
	db.clientOptions = clientOptions
	tried := make(chan bool)
	go db.start(registry, tried)
	<-tried
	return nil
}

type exitFn func(int)

var exitHook exitFn = os.Exit

// Terminate will close the connection to Mongo
func (db *mongoDB) Terminate() {
	if db.state != DBNotConnected {
		db.mux.Lock()
		defer db.mux.Unlock()
		if db.state != DBNotConnected {
			db.state = DBNotConnected
			db.Log.Info("MongoDB state ⇒ NotConnected")
			db.Log.Info("Closing connection to mongo")
			ctx, cancel := context.WithTimeout(context.Background(), db.Timeout)
			defer cancel()
			if err := db.client.Disconnect(ctx); err != nil {
				db.Log.Criticalf("Failed to disconnect mongo: %s", err.Error())
				exitHook(1)
			}
			db.client = nil
		}
	}
}

// DBClient methods

// DBName returns the name of the database
func (db *mongoDB) DBName() string {
	return db.DatabaseName
}

// DBTimeout returns the timeout for database operations
func (db *mongoDB) DBTimeout() time.Duration {
	return db.Timeout
}

// Client returns the client interface
func (db *mongoDB) Client() Client {
	return db.client
}

// Logger returns the logger
func (db *mongoDB) Logger() *logging.Logger {
	return db.Log
}

// ErrorCode extracts the numeric mongo error codes
func (db *mongoDB) ErrorCode(err error) int {
	if err == nil {
		// used to reset the state of the MongoDB to DBReady
		db.becomeReady(false)
		return ECKeyNotFound
	}
	code := ECUnknownError
	switch e := err.(type) {
	case mongo.CommandError: // returned from the database service
		db.becomeReady(false)
		return int(e.Code)
	case mongo.WriteException: // returned by the database service
		db.becomeReady(false)
		if e.WriteConcernError != nil {
			// based to comments in the mongo driver, WriteConcernError should be reported over write, but log if both are present
			if len(e.WriteErrors) > 0 {
				db.Log.Infof("ErrorCode: WriteException %s", err.Error())
			}
			return e.WriteConcernError.Code
		} else if len(e.WriteErrors) > 0 {
			// return the first error code, log if there is more than one
			if len(e.WriteErrors) > 1 {
				db.Log.Infof("ErrorCode: WriteException %s", err.Error())
			}
			return e.WriteErrors[0].Code
		}
		// code is still ECUnknownError, log below
	case *net.OpError:
		db.Log.Infof("ErrorCode: OpError %s", e.Error())
		code = ECSocketException
	default:
		if err == mongo.ErrNoDocuments { // returned by SingleResult.Decode() when no result
			db.becomeReady(false)
			return ECKeyNotFound
		} else if err == io.EOF { // returned by any op when socket disconnects during a read
			code = ECInterrupted
		} else if err == context.DeadlineExceeded {
			code = ECInterrupted
		} else if err == mongo.ErrUnacknowledgedWrite || err == mongo.ErrClientDisconnected {
			code = ECInterrupted
		} else if strings.HasPrefix(err.Error(), "server selection error") { // returned by any op when mongo is not reachable
			code = ECHostUnreachable
		}
	}
	if code == ECUnknownError {
		db.Log.Infof("ErrorCode: %T %s", err, err.Error())
	} else {
		// other codes indicate some sort of network connection problem, change state
		if db.state == DBReady {
			db.mux.Lock()
			defer db.mux.Unlock()
			if db.state == DBReady {
				db.state = DBError
				db.Log.Error("MongoDB state ⇒ Error")
			}
		}
	}
	return code
}

// MustBeInitializing fails if database is not initializing
func (db *mongoDB) MustBeInitializing() error {
	if db.state != DBInitializing {
		db.Log.Error("MongoDB: not in initializing state")
		return fmt.Errorf("database not initializing")
	}
	return nil
}

// MustBeReady fails if database initialization is not complete
func (db *mongoDB) MustBeReady() error {
	if db.state < DBReady {
		db.Log.Error("MongoDB: not ready")
		return fmt.Errorf("database not available")
	}
	return nil
}

// MongoClient wraps the mongo.Client struct
type MongoClient struct {
	*mongo.Client
}

var _ = Client(&MongoClient{})

// Database wraps the mongo.Client.Database function
func (mc *MongoClient) Database(name string, opts ...*options.DatabaseOptions) Database {
	return &MongoDatabase{mc.Client.Database(name, opts...)}
}

// Disconnect wraps the mongo.Client.Disconnect function
func (mc *MongoClient) Disconnect(ctx context.Context) error {
	return mc.Client.Disconnect(ctx)
}

// Ping wraps the mongo.Client.Ping function
func (mc *MongoClient) Ping(ctx context.Context, rp *readpref.ReadPref) error {
	return mc.Client.Ping(ctx, rp)
}

// MongoDatabase wraps a mongo.Database to embed functions in an interface
type MongoDatabase struct {
	*mongo.Database
}

var _ = Database(&MongoDatabase{})

// Collection wraps the mongo.Database.Collection function
func (md *MongoDatabase) Collection(name string, opts ...*options.CollectionOptions) Collection {
	return &MongoCollection{md.Database.Collection(name, opts...)}
}

// RunCommand wraps the mongo.Database.RunCommand function
func (md *MongoDatabase) RunCommand(ctx context.Context, runCommand interface{}, opts ...*options.RunCmdOptions) SingleResult {
	return &MongoSingleResult{md.Database.RunCommand(ctx, runCommand, opts...)}
}

// MongoCollection wraps a mongo.Collection to embed functions in an interface
type MongoCollection struct {
	*mongo.Collection
}

var _ = Collection(&MongoCollection{})

// Aggregate wraps the mongo.Collection.Aggregate function
func (mc *MongoCollection) Aggregate(ctx context.Context, pipeline interface{}, opts ...*options.AggregateOptions) (Cursor, error) {
	return mc.Collection.Aggregate(ctx, pipeline, opts...)
}

// Find wraps the mongo.Collection.Find function
func (mc *MongoCollection) Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (Cursor, error) {
	return mc.Collection.Find(ctx, filter, opts...)
}

// MongoSingleResult wraps a mongo.SingleResult to embed functions in an interface
type MongoSingleResult struct {
	*mongo.SingleResult
}

// FindOne wraps the mongo.Collection.FindOne function
func (mc *MongoCollection) FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) SingleResult {
	return &MongoSingleResult{mc.Collection.FindOne(ctx, filter, opts...)}
}

// FindOneAndUpdate wraps the mongo.Collection.FindOneAndUpdate function
func (mc *MongoCollection) FindOneAndUpdate(ctx context.Context, filter interface{}, update interface{}, opts ...*options.FindOneAndUpdateOptions) SingleResult {
	return &MongoSingleResult{mc.Collection.FindOneAndUpdate(ctx, filter, update, opts...)}
}

// MongoIndexView wraps a mongo.IndexView to embed functions in an interface
type MongoIndexView struct {
	mongo.IndexView
}

var _ = IndexView(&MongoIndexView{})

// Indexes wraps the mongo.Collection.Indexes function
func (mc *MongoCollection) Indexes() IndexView {
	return &MongoIndexView{mc.Collection.Indexes()}
}
