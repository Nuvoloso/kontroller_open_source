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


package pgdb

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"strings"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	logging "github.com/op/go-logging"
)

// Postgres driver connection defaults
const (
	DefaultHost           = "localhost"
	DefaultPort     int16 = 5432
	DefaultUser           = "postgres"
	DefaultDatabase       = "postgres"
)

// DBArgs contains the externally visible arguments to access the database.
type DBArgs struct {
	Host       string
	Port       int16
	User       string
	Database   string
	DebugLevel int

	// properties related to SSL/TLS
	UseSSL            bool
	TLSCertificate    string
	TLSCertificateKey string
	TLSCACertificate  string
	TLSServerName     string
}

func (dba *DBArgs) init() {
	if dba.Host == "" {
		dba.Host = DefaultHost
	}
	if dba.Port == 0 {
		dba.Port = DefaultPort
	}
	if dba.User == "" {
		dba.User = DefaultUser
	}
	if dba.Database == "" {
		dba.Database = DefaultDatabase
	}
}

// ErrorDesc describes common errors
type ErrorDesc int

// ErrorDesc values
const (
	ErrUnknown ErrorDesc = iota
	ErrDatabaseExists
	ErrConnectionError
)

// DB is an interface to:
// - open a Postgres database (with the PGX driver)
// - decode postgres specific errors
type DB interface {
	OpenDB(args *DBArgs) (*sql.DB, error)
	PingContext(ctx context.Context, db *sql.DB) error
	SQLCode(err error) string
	ErrDesc(err error) ErrorDesc
	SetSQLOpener(SQLOpener) // to enable mocking of SQL in UTs
}

// SQLOpener matches the signature of the sql.Open call
type SQLOpener interface {
	Open(driverName, dataSourceName string) (*sql.DB, error)
}

// New returns a Database interface to get a sql.DB with a PGX driver
func New(log *logging.Logger) DB {
	pg := &pgDB{}
	pg.log = log
	pg.SetSQLOpener(pg) // self-reference
	return pg
}

// pgDB configures the Postgres Pgx driver
type pgDB struct {
	DBArgs
	log     *logging.Logger
	lastMsg string
	opener  SQLOpener
	db      *sql.DB
}

// OpenDB returns a database connection
func (pg *pgDB) OpenDB(args *DBArgs) (*sql.DB, error) {
	pg.DBArgs = *args
	pg.DBArgs.init()
	pg.DebugLevel = 2 // XXX Temporary
	var tlsConfig *tls.Config
	sslMode := ""
	if args.UseSSL {
		tlsClientOpts := httptransport.TLSClientOptions{
			Certificate: pg.TLSCertificate,
			Key:         pg.TLSCertificateKey,
			CA:          pg.TLSCACertificate,
			ServerName:  pg.TLSServerName,
		}
		var err error
		if tlsConfig, err = httptransport.TLSClientAuth(tlsClientOpts); err != nil {
			return nil, err
		}
		// Nonintuitively, this causes our tlsConfig to be used as is, else our tlsConfig is overridden by one derived from the sslmode in the DSN
		sslMode = " sslmode=disable"
	}
	connString := fmt.Sprintf("host=%s port=%d user=%s database=%s%s", pg.Host, pg.Port, pg.User, pg.Database, sslMode)
	driverConfig := &stdlib.DriverConfig{
		ConnConfig: pgx.ConnConfig{
			Logger:    pg, // implements pgx.Logger
			TLSConfig: tlsConfig,
		},
	}
	stdlib.RegisterDriverConfig(driverConfig)
	return pg.opener.Open("pgx", driverConfig.ConnectionString(connString))
}

// Log matches the pgx.Logger interface
func (pg *pgDB) Log(ll pgx.LogLevel, msg string, data map[string]interface{}) {
	switch pg.DebugLevel {
	case 0:
		return
	case 1:
		if ll >= pgx.LogLevelWarn {
			return
		}
	}
	if msg == pg.lastMsg {
		return
	}
	pg.lastMsg = msg
	args := []string{}
	for k, v := range data {
		args = append(args, fmt.Sprintf("%s: %#v", k, v))
	}
	pg.log.Debugf("%s [%s]", msg, strings.Join(args, ", "))
}

// SQLCode returns the SQL database code
func (pg *pgDB) SQLCode(err error) string {
	return SQLCode(err)
}

// ErrDesc returns the error descriptor
func (pg *pgDB) ErrDesc(err error) ErrorDesc {
	ed, code := ErrDesc(err)
	if ed == ErrUnknown && code != "" {
		pg.log.Debugf("Postgres error: %s", code)
	}
	return ed
}

// SQLCode returns the SQL database code
func SQLCode(err error) string {
	e, ok := err.(pgx.PgError)
	if !ok {
		return ""
	}
	return e.Code
}

// ErrDesc returns the error descriptor and SQL code
func ErrDesc(err error) (ErrorDesc, string) {
	code := SQLCode(err)
	if strings.HasPrefix(code, "08") {
		return ErrConnectionError, code
	}
	switch code {
	case "42P04":
		return ErrDatabaseExists, code
	}
	return ErrUnknown, code
}

// Open invokes the real sql.Open function
func (pg *pgDB) Open(driverName, dataSourceName string) (*sql.DB, error) {
	return sql.Open(driverName, dataSourceName)
}

// SetSQLOpener sets the SQLOpener interface
func (pg *pgDB) SetSQLOpener(o SQLOpener) {
	pg.opener = o
}

// PingContext wraps the call to PingContext because sqlmock does not provide expectations on this method.
func (pg *pgDB) PingContext(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}
