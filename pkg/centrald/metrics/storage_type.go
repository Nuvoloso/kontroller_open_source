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


package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// StorageTypeData contains cached information on a storage type
type StorageTypeData struct {
	ResponseTimeMeanUsec int32
	ResponseTimeMaxUsec  int32
	IOPsPerGiB           int32
	BytesPerSecPerGiB    int64
}

// updateStorageTypeData updates the StorageTypeData table on first connection
func (c *Component) updateStorageTypeData(ctx context.Context) error {
	var err error
	// prepare an upsert statement using a versioned stored procedure
	var stmt *sql.Stmt
	ver := c.TableVersion("StorageTypeData")
	switch ver {
	case 1:
		stmt, err = c.SafePrepare(ctx, "SELECT StorageTypeDataUpsert1($1, $2, $3, $4, $5)")
	default:
		err = fmt.Errorf("unsupported StorageTypeData version %d", ver)
	}
	if err != nil {
		return err
	}
	defer stmt.Close()
	// convert response time values to microseconds (max in db ~ 30min)
	sscResponseTimeUSec := func(st *models.CSPStorageType, sn string) int32 {
		if ssc, ok := st.SscList.SscListMutable[sn]; ok {
			if d, err := time.ParseDuration(ssc.Value); err == nil {
				return int32(d / time.Microsecond)
			}
		}
		return 0
	}
	// upsert storage type data
	for _, st := range c.App.AppCSP.SupportedCspStorageTypes() {
		rtMean := sscResponseTimeUSec(st, SscResponseTimeAverage)
		rtMax := sscResponseTimeUSec(st, SscResponseTimeMaximum)
		iopsPerGiB := int32(swag.Int64Value(st.ProvisioningUnit.IOPS))
		bpsPerGiB := swag.Int64Value(st.ProvisioningUnit.Throughput)
		if ver == 1 {
			_, err := stmt.ExecContext(ctx, string(st.Name), rtMean, rtMax, iopsPerGiB, bpsPerGiB)
			if err != nil {
				err = fmt.Errorf("%s (%s)", err.Error(), c.pgDB.SQLCode(err))
				return err
			}
		}
		std := &StorageTypeData{
			ResponseTimeMeanUsec: rtMean,
			ResponseTimeMaxUsec:  rtMax,
			IOPsPerGiB:           iopsPerGiB,
			BytesPerSecPerGiB:    bpsPerGiB,
		}
		c.cacheStorageType(string(st.Name), std)
	}
	c.storageTypeDataUpdated = true
	return nil
}

func (c *Component) cacheStorageType(stID string, std *StorageTypeData) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.storageTypeCache == nil {
		c.storageTypeCache = make(map[string]*StorageTypeData)
	}
	c.storageTypeCache[stID] = std
}

func (c *Component) lookupStorageType(stID string) *StorageTypeData {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.storageTypeCache != nil {
		if std, ok := c.storageTypeCache[stID]; ok {
			return std
		}
	}
	return nil
}
