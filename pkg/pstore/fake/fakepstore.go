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

	"github.com/Nuvoloso/kontroller/pkg/pstore"
)

// Controller is a fake pstore Controller
type Controller struct {
	// SnapshotBackup
	InSBArg  *pstore.SnapshotBackupArgs
	RetSBObj *pstore.SnapshotBackupResult
	RetSBErr error

	// SnapshotDelete
	InSDArg  *pstore.SnapshotDeleteArgs
	RetSDObj *pstore.SnapshotDeleteResult
	RetSDErr error

	// SnapshotRestore
	InSRArg  *pstore.SnapshotRestoreArgs
	RetSRObj *pstore.SnapshotRestoreResult
	RetSRErr error

	// SnapshotCatalogUpsert
	InSCatUpArg  *pstore.SnapshotCatalogUpsertArgs
	RetSCatUpErr error
}

var _ = pstore.Operations(&Controller{})

func recastError(err error) error {
	if err != nil {
		if e, ok := err.(pstore.Error); ok {
			err = e
		} else {
			err = pstore.NewFatalError(err.Error())
		}
	}
	return err
}

// SnapshotBackup fakes its namesake
func (c *Controller) SnapshotBackup(ctx context.Context, args *pstore.SnapshotBackupArgs) (*pstore.SnapshotBackupResult, error) {
	c.InSBArg = args
	return c.RetSBObj, recastError(c.RetSBErr)
}

// SnapshotDelete fakes its namesake
func (c *Controller) SnapshotDelete(ctx context.Context, args *pstore.SnapshotDeleteArgs) (*pstore.SnapshotDeleteResult, error) {
	c.InSDArg = args
	return c.RetSDObj, recastError(c.RetSDErr)
}

// SnapshotRestore fakes its namesake
func (c *Controller) SnapshotRestore(ctx context.Context, args *pstore.SnapshotRestoreArgs) (*pstore.SnapshotRestoreResult, error) {
	c.InSRArg = args
	return c.RetSRObj, recastError(c.RetSRErr)
}

// SnapshotCatalogList fakes its namesake
func (c *Controller) SnapshotCatalogList(ctx context.Context, args *pstore.SnapshotCatalogListArgs) (pstore.SnapshotCatalogIterator, error) {
	return nil, nil
}

// SnapshotCatalogUpsert fakes its namesake
func (c *Controller) SnapshotCatalogUpsert(ctx context.Context, args *pstore.SnapshotCatalogUpsertArgs) (*pstore.SnapshotUpsertResult, error) {
	c.InSCatUpArg = args
	return nil, c.RetSCatUpErr
}

// SnapshotCatalogGet fakes its namesake
func (c *Controller) SnapshotCatalogGet(ctx context.Context, args *pstore.SnapshotCatalogGetArgs) (*pstore.SnapshotCatalogEntry, error) {
	return nil, nil
}
