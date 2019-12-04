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
	"context"
	"database/sql"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	pgdb "github.com/Nuvoloso/kontroller/pkg/pgdb"
)

// DB is a fake PGDB
type DB struct {
	Mock sqlmock.Sqlmock

	// OpenDB
	RetODErr error

	// PingContext
	InPCDb   *sql.DB
	RetPCErr error
}

var _ = pgdb.DB(&DB{})

// OpenDB provides a sqlmock database
func (pg *DB) OpenDB(args *pgdb.DBArgs) (*sql.DB, error) {
	if pg.RetODErr != nil {
		return nil, pg.RetODErr
	}
	db, mock, err := sqlmock.New()
	pg.Mock = mock
	return db, err
}

// SQLCode fakes its namesake
func (pg *DB) SQLCode(err error) string {
	return ""
}

// ErrDesc fakes its namesake
func (pg *DB) ErrDesc(err error) pgdb.ErrorDesc {
	ed, _ := pgdb.ErrDesc(err)
	return ed
}

// SetSQLOpener fakes its namesake
func (pg *DB) SetSQLOpener(o pgdb.SQLOpener) {
}

// PingContext fakes its namesake
func (pg *DB) PingContext(ctx context.Context, db *sql.DB) error {
	pg.InPCDb = db
	return pg.RetPCErr
}
