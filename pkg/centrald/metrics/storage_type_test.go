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
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appmock "github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestUpdateStorageTypeData(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c := Component{}
	c.Init(app)
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg
	var err error
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true

	stID := "Amazon gp2"
	sts := []*models.CSPStorageType{
		&models.CSPStorageType{
			Name: models.CspStorageType(stID),
			SscList: &models.SscList{SscListMutable: models.SscListMutable{
				"Response Time Average": {Kind: "DURATION", Value: "5ms"},
				"Response Time Maximum": {Kind: "DURATION", Value: "200ms"},
				"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
			}},
			ProvisioningUnit: &models.ProvisioningUnit{
				IOPS:       swag.Int64(int64(3)),
				Throughput: swag.Int64(0),
			},
		},
		&models.CSPStorageType{
			Name: "foo",
			SscList: &models.SscList{SscListMutable: models.SscListMutable{
				//"Response Time Average": {Kind: "DURATION", Value: "5ms"}, // deliberately empty to exercise code path
				//"Response Time Maximum": {Kind: "DURATION", Value: "5ms"}, // deliberately empty to exercise code path
				"Availability": {Kind: "PERCENTAGE", Value: "99.9%"},
			}},
			ProvisioningUnit: &models.ProvisioningUnit{},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	appCSP := appmock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().SupportedCspStorageTypes().Return(sts).MinTimes(1)

	// invalid table version
	c.tableVersions = nil
	assert.Equal(0, c.TableVersion("StorageTypeData"))
	err = c.updateStorageTypeData(ctx)
	assert.Error(err)
	assert.Regexp("unsupported StorageTypeData version", err)
	assert.False(c.storageTypeDataUpdated)

	// prepare error
	c.tableVersions = map[string]int{"StorageTypeData": 1}
	assert.Equal(1, c.TableVersion("StorageTypeData"))
	pg.Mock.ExpectPrepare("StorageTypeDataUpsert1").
		WillReturnError(fmt.Errorf("prepare-error"))
	err = c.updateStorageTypeData(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.False(c.storageTypeDataUpdated)

	// exec error
	rtMean := int32(5 * time.Millisecond / time.Microsecond)
	rtMax := int32(200 * time.Millisecond / time.Microsecond)
	iopsPerGiB := int32(3)
	bytesPerSecPerGiB := int64(0) // not in plan
	pg.Mock.ExpectPrepare("StorageTypeDataUpsert1.*1.*2.*3.*4.*5").
		ExpectExec().
		WithArgs(stID, rtMean, rtMax, iopsPerGiB, bytesPerSecPerGiB).
		WillReturnError(fmt.Errorf("exec-error"))
	err = c.updateStorageTypeData(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("exec-error", err)
	assert.False(c.storageTypeDataUpdated)

	// no errors
	sqlRes := sqlmock.NewResult(1, 1)
	pg.Mock.ExpectPrepare("StorageTypeDataUpsert1.*1.*2.*3.*4.*5").
		ExpectExec().
		WithArgs(stID, rtMean, rtMax, iopsPerGiB, bytesPerSecPerGiB).
		WillReturnResult(sqlRes)
	sqlRes = sqlmock.NewResult(1, 1)
	pg.Mock.ExpectExec(".*").
		WithArgs("foo", 0, 0, 0, 0).
		WillReturnResult(sqlRes)
	assert.Nil(c.storageTypeCache)
	assert.Nil(c.lookupStorageType(stID))
	err = c.updateStorageTypeData(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.True(c.storageTypeDataUpdated)
	assert.NotEmpty(c.storageTypeCache)
	std := c.lookupStorageType(stID)
	assert.NotNil(std)
	assert.Equal(rtMean, std.ResponseTimeMeanUsec)
	assert.Equal(rtMax, std.ResponseTimeMaxUsec)
}
