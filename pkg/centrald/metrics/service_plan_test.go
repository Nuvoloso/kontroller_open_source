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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestUpdateServicePlans(t *testing.T) {
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
	fc := &fake.Client{}
	c.oCrud = fc

	assert.False(c.servicePlanDataUpdated)

	// service plan list error
	fc.RetLsSvPErr = fmt.Errorf("service-plan-list-error")
	err = c.updateServicePlanData(ctx)
	assert.Error(err)
	assert.Regexp("service-plan-list-error", err)
	assert.False(c.servicePlanDataUpdated)

	// invalid table version
	planID := "80ceb62c-671d-4d07-9abd-3688808aa704"
	lRes := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{ID: models.ObjID(planID)},
					ProvisioningUnit: &models.ProvisioningUnit{
						IOPS: swag.Int64(1000),
					},
					IoProfile: &models.IoProfile{
						IoPattern: &models.IoPattern{
							MinSizeBytesAvg: swag.Int32(0),
							MaxSizeBytesAvg: swag.Int32(16384),
						},
						ReadWriteMix: &models.ReadWriteMix{
							MinReadPercent: swag.Int32(30),
							MaxReadPercent: swag.Int32(70),
						},
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Slos: map[string]models.RestrictedValueType{
						SloResponseTimeAverage: models.RestrictedValueType{
							ValueType: models.ValueType{
								Value: "10ms",
							},
						},
						// no max response time to exercise code path
					},
				},
			},
		},
	}
	fc.RetLsSvPErr = nil
	fc.RetLsSvPObj = lRes
	c.tableVersions = nil
	assert.Equal(0, c.TableVersion("ServicePlanData"))
	err = c.updateServicePlanData(ctx)
	assert.Error(err)
	assert.Regexp("unsupported ServicePlanData version", err)
	assert.False(c.servicePlanDataUpdated)

	// prepare error
	c.tableVersions = map[string]int{"ServicePlanData": 1}
	assert.Equal(1, c.TableVersion("ServicePlanData"))
	pg.Mock.ExpectPrepare("ServicePlanDataUpsert1.*1.*2.*3.*4.*5.*6.*7.*8.*9").
		WillReturnError(fmt.Errorf("prepare-error"))
	err = c.updateServicePlanData(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.False(c.servicePlanDataUpdated)

	// exec error
	rt := int32(10 * time.Millisecond / time.Microsecond)
	rtMax := int32(0) // not in plan
	iopsPerGiB := int32(1000)
	bytesPerSecPerGiB := int64(0) // not in plan
	minReadPct := int32(30)
	maxReadPct := int32(70)
	minAvgSizeBytes := int32(0)
	maxAvgSizeBytes := int32(16384)
	pg.Mock.ExpectPrepare("ServicePlanDataUpsert1.*1.*2.*3.*4.*5.*6.*7.*8.*9").
		ExpectExec().
		WithArgs(planID, rt, rtMax, iopsPerGiB, bytesPerSecPerGiB, minReadPct, maxReadPct, minAvgSizeBytes, maxAvgSizeBytes).
		WillReturnError(fmt.Errorf("exec-error"))
	err = c.updateServicePlanData(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("exec-error", err)
	assert.False(c.servicePlanDataUpdated)

	// no errors
	sqlRes := sqlmock.NewResult(1, 1)
	pg.Mock.ExpectPrepare("ServicePlanDataUpsert1.*1.*2.*3.*4.*5.*6.*7.*8.*9").
		ExpectExec().
		WithArgs(planID, rt, rtMax, iopsPerGiB, bytesPerSecPerGiB, minReadPct, maxReadPct, minAvgSizeBytes, maxAvgSizeBytes).
		WillReturnResult(sqlRes)
	assert.Nil(c.servicePlanCache)
	assert.Nil(c.lookupServicePlan(planID))
	err = c.updateServicePlanData(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.True(c.servicePlanDataUpdated)
	assert.NotEmpty(c.servicePlanCache)
	spd := c.lookupServicePlan(planID)
	assert.NotNil(spd)
	assert.Equal(rt, spd.ResponseTimeMeanUsec)
	assert.Equal(rtMax, spd.ResponseTimeMaxUsec)
}
