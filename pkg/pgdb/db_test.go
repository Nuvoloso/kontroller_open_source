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
	"database/sql"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
)

func TestDBArgs(t *testing.T) {
	assert := assert.New(t)

	args := &DBArgs{}
	args.init()
	assert.Equal(DefaultHost, args.Host)
	assert.Equal(DefaultPort, args.Port)
	assert.Equal(DefaultUser, args.User)
	assert.Equal(DefaultDatabase, args.Database)

	args = &DBArgs{
		Host:     "somehost",
		Port:     1234,
		User:     "someuser",
		Database: "someDB",
	}
	args.init()
	assert.Equal("somehost", args.Host)
	assert.Equal(int16(1234), args.Port)
	assert.Equal("someuser", args.User)
	assert.Equal("someDB", args.Database)

	pg := &pgDB{}
	pg.DBArgs.init()
	args = &pg.DBArgs
	assert.Equal(DefaultHost, args.Host)
	assert.Equal(DefaultPort, args.Port)
	assert.Equal(DefaultUser, args.User)
	assert.Equal(DefaultDatabase, args.Database)
}

func TestLog(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	pgdb := New(tl.Logger())
	pg, ok := pgdb.(*pgDB)
	assert.True(ok)
	assert.Equal(0, pg.DebugLevel)
	pg.Log(pgx.LogLevelWarn, "NOTLOGGED debug is off", nil)
	pg.DebugLevel = 1
	pg.Log(pgx.LogLevelWarn, "NOTLOGGED Warn not logged", nil)
	pg.Log(pgx.LogLevelInfo, "NOTLOGGED Info not logged", nil)
	pg.Log(pgx.LogLevelDebug, "NOTLOGGED Debug not logged", nil)
	pg.Log(pgx.LogLevelTrace, "NOTLOGGED Trace not logged", nil)
	pg.Log(pgx.LogLevelError, "LOGGED Error is logged", nil)
	pg.Log(pgx.LogLevelNone, "LOGGED None is logged", nil)
	assert.Equal(0, tl.CountPattern("NOTLOGGED"))
	assert.Equal(2, tl.CountPattern("LOGGED"))
	tl.Flush()

	// test keys and suppression of repeated msg (not keys)
	data := map[string]interface{}{}
	data["intkey"] = 1
	data["stringkey"] = "stringval"
	pg.Log(pgx.LogLevelError, "msg with data", data)
	assert.Equal(1, tl.CountPattern("msg with data"))
	assert.Equal(1, tl.CountPattern("intkey: 1"))
	assert.Equal(1, tl.CountPattern("stringkey: \"stringval\""))
	assert.Equal("msg with data", pg.lastMsg) // note not the map values

	pg.Log(pgx.LogLevelError, "msg with data", data)
	assert.Equal(1, tl.CountPattern("msg with data"))
	assert.Equal(1, tl.CountPattern("intkey: 1"))
	assert.Equal(1, tl.CountPattern("stringkey: \"stringval\""))
}

func TestOpen(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	pgdb := New(tl.Logger())
	pg, ok := pgdb.(*pgDB)
	assert.True(ok)
	assert.Equal(pg, pg.opener) // self reference

	// intercept the sql.Open call
	fso := &fakeSQLOpener{}
	pgdb.SetSQLOpener(fso)

	// fail the open, cover successful UseSSL path
	fso.sqlOpenErr = fmt.Errorf("sql-open-error")
	args := &DBArgs{
		UseSSL:           true,
		TLSCACertificate: "../mgmtclient/ca.crt",
		TLSServerName:    "set",
	}
	db, err := pg.OpenDB(args)
	assert.Error(err)
	assert.Regexp("sql-open-error", err)
	assert.Nil(db)
	assert.Equal("pgx", fso.sqlDN)
	tl.Logger().Infof("DSN: %s", fso.sqlDSN)
	dsnPat := fmt.Sprintf("host=%s port=%d user=%s database=%s sslmode=disable", pg.Host, pg.Port, pg.User, pg.Database)
	assert.Regexp(dsnPat, fso.sqlDSN)

	// UseSSL fails creating tlsConfig
	args.TLSCACertificate = "invalid.crt"
	db, err = pg.OpenDB(args)
	assert.Nil(db)
	assert.Regexp("no such file or directory", err)

	// succeed, No SSL
	args.UseSSL = false
	sqlDB := &sql.DB{}
	fso.sqlDB = sqlDB
	fso.sqlOpenErr = nil
	db, err = pg.OpenDB(args)
	assert.NoError(err)
	assert.NotNil(db)
	dsnPat = fmt.Sprintf("host=%s port=%d user=%s database=%s$", pg.Host, pg.Port, pg.User, pg.Database)
	assert.Regexp(dsnPat, fso.sqlDSN)

	// now call the real SQL open
	pgdb = New(tl.Logger())
	db, err = pgdb.OpenDB(args)
	assert.NoError(err)
	assert.NotNil(db)
	pg, ok = pgdb.(*pgDB)
	assert.True(ok)
	args = &pg.DBArgs
	assert.Equal(DefaultHost, args.Host)
	assert.Equal(DefaultPort, args.Port)
	assert.Equal(DefaultUser, args.User)
	assert.Equal(DefaultDatabase, args.Database)
}

type fakeSQLOpener struct {
	sqlDB         *sql.DB
	sqlOpenErr    error
	sqlDN, sqlDSN string
}

func (fso *fakeSQLOpener) Open(driverName, dataSourceName string) (*sql.DB, error) {
	fso.sqlDN = driverName
	fso.sqlDSN = dataSourceName
	if fso.sqlOpenErr != nil {
		return nil, fso.sqlOpenErr
	}
	return fso.sqlDB, nil
}

func TestErrorConversion(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	pgdb := New(tl.Logger())

	pgxE := pgx.PgError{Code: "42P04"}
	assert.Equal("42P04", pgdb.SQLCode(pgxE))
	assert.Equal(ErrDatabaseExists, pgdb.ErrDesc(pgxE))

	for _, s := range []string{"08000", "08001", "08003", "08004", "08006", "08007", "08P01"} {
		pgxE = pgx.PgError{Code: s}
		assert.Equal(s, pgdb.SQLCode(pgxE))
		assert.Equal(ErrConnectionError, pgdb.ErrDesc(pgxE))
	}

	err := fmt.Errorf("other-error")
	assert.Equal("", pgdb.SQLCode(err))
	assert.Equal(ErrUnknown, pgdb.ErrDesc(err))

	pgxE = pgx.PgError{Code: "NotHandled"}
	assert.Equal("NotHandled", pgdb.SQLCode(pgxE))
	assert.Equal(ErrUnknown, pgdb.ErrDesc(pgxE))
	assert.Equal(1, tl.CountPattern("Postgres error: NotHandled"))
}

func TestUsageWithMock(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	pgdb := New(tl.Logger())
	mso := &mockSQLOpener{}
	pgdb.SetSQLOpener(mso)

	db, err := pgdb.OpenDB(&DBArgs{})
	assert.NoError(err)
	assert.NotNil(db)
	assert.NotNil(mso.mock)
	defer db.Close()

	err = pgdb.PingContext(ctx, db) // no mock expectations
	assert.NoError(err)

	mso.mock.ExpectExec("SELECT something").WithArgs(1).WillReturnError(fmt.Errorf("db-exec-err"))
	_, err = db.ExecContext(ctx, "SELECT something()", 1)
	assert.Error(err)
	assert.Regexp("db-exec-err", err)
	assert.NoError(mso.mock.ExpectationsWereMet())

	mRows := sqlmock.NewRows([]string{"id", "title"}).
		AddRow(1, "one").
		AddRow(2, "two")
	mso.mock.ExpectQuery("SELECT \\* FROM SomeTable").WithArgs(1).WillReturnRows(mRows)
	rs, err := db.QueryContext(ctx, "SELECT * FROM SomeTable", 1)
	assert.NoError(err)
	assert.NotNil(rs)
	defer rs.Close()

	got := map[int]string{}
	for rs.Next() {
		var id int
		var title string
		rs.Scan(&id, &title)
		got[id] = title
	}
	assert.Equal(map[int]string{1: "one", 2: "two"}, got)
	assert.NoError(rs.Err())

	assert.NoError(mso.mock.ExpectationsWereMet())

}

type mockSQLOpener struct {
	mock sqlmock.Sqlmock
}

func (mso *mockSQLOpener) Open(driverName, dataSourceName string) (*sql.DB, error) {
	db, mock, err := sqlmock.New()
	mso.mock = mock
	return db, err
}
