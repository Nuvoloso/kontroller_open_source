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
	"time"

	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// The interface to mongo is abstracted to support testing. The following main abstractions are defined:
//   - DBAPI interface
//     This is an abstraction of the API that serves as:
//       - the means to hide implementation details on error codes
//       - the mechanism for document handlers to interact with their external environment
//   - ObjectDocumentHandler interface
//     The actual database interaction in the datastore is carried out by object document handlers
//     that have to work with both the real as well as the mock database so they must be defined
//     independent of either and get linked to their runtime environment via their interfaces.
//     A registry of object document handlers is defined.
//   - Mongo API wrapper interfaces
//     This is an identical facsimile of the required parts of the mongo API interfaces.
//     Note that data types referenced are mongo client data types.

// DBAPI abstracts the database API
type DBAPI interface {
	// Connect starts the mongodb client engine in a separate go-routine.
	// The registry is used by this go-routine to Initialize and Start each ObjectDocumentHandler
	Connect(registry ObjectDocumentHandlerMap) error
	// Terminate terminates the mongodb client engine
	Terminate()
	// DBName returns the name of the database
	DBName() string
	// DBTimeout returns the timeout for database operations
	DBTimeout() time.Duration
	// Client returns the client interface
	Client() Client
	// Logger returns a logger
	Logger() *logging.Logger
	// MustBeInitializing fails if database is not initializing
	MustBeInitializing() error
	// MustBeReady returns an error if database initialization is not complete
	MustBeReady() error
	// This method can extract the error code from an error
	ErrorCode(error) int
}

// ObjectDocumentHandler describes a handler for an object represented by a Mongo document
type ObjectDocumentHandler interface {
	// Initialize is called to initialize the object handler and perform any isolated
	// database setup activity prior to object modification operations across any handlers.
	// The handler should stash the DBAPI.
	Initialize(context.Context) error
	// Start is called when the initialization of all object handlers has completed.
	// The document handler can use this method to establish initial state.
	Start(context.Context) error
	// CName returns the name of the collection.
	CName() string
}

// ObjectDocumentHandlerMap is a map of document handler name to ObjectDocumentHandler
type ObjectDocumentHandlerMap map[string]ObjectDocumentHandler

// Mongo error codes of interest
const (
	ECHostUnreachable = 6
	ECUnknownError    = 8
	ECNamespaceExists = 48
	ECKeyNotFound     = 211
	ECSocketException = 9001
	ECDuplicateKey    = 11000
	ECInterrupted     = 11601
)

// The interfaces below are based on mongo API semantics

// Client is an interface to access to the datastore client
type Client interface {
	Database(string, ...*options.DatabaseOptions) Database
	Disconnect(context.Context) error
	Ping(context.Context, *readpref.ReadPref) error
}

// Database is an interface to access to the datastore
type Database interface {
	Collection(string, ...*options.CollectionOptions) Collection
	RunCommand(context.Context, interface{}, ...*options.RunCmdOptions) SingleResult
}

// Collection is an interface to access to a collection.
// Interface is sparse - only required functions are provided.
type Collection interface {
	Aggregate(context.Context, interface{}, ...*options.AggregateOptions) (Cursor, error)
	CountDocuments(context.Context, interface{}, ...*options.CountOptions) (int64, error)
	DeleteOne(context.Context, interface{}, ...*options.DeleteOptions) (*mongo.DeleteResult, error)
	Find(context.Context, interface{}, ...*options.FindOptions) (Cursor, error)
	FindOne(context.Context, interface{}, ...*options.FindOneOptions) SingleResult
	FindOneAndUpdate(context.Context, interface{}, interface{}, ...*options.FindOneAndUpdateOptions) SingleResult
	Indexes() IndexView
	InsertOne(context.Context, interface{}, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
	UpdateMany(context.Context, interface{}, interface{}, ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

// Cursor abstracts the mongo Cursor struct
// Note that the argument to the Decode() function must be a pointer, for example &bson.M{}, not bson.M{}
type Cursor interface {
	Close(context.Context) error
	Decode(interface{}) error
	Err() error
	Next(context.Context) bool
}

// IndexView abstracts the mongo IndexView struct
type IndexView interface {
	CreateOne(context.Context, mongo.IndexModel, ...*options.CreateIndexesOptions) (string, error)
}

// SingleResult abstracts the SingleResult struct
// Note that the argument to the Decode() function must be a pointer, for example &bson.M{}, not bson.M{}
type SingleResult interface {
	Decode(interface{}) error
	Err() error
}
