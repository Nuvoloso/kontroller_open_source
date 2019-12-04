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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// ServicePlanData contains cached information on a service plan
type ServicePlanData struct {
	ResponseTimeMeanUsec int32
	ResponseTimeMaxUsec  int32
	IOPsPerGiB           int32
	BytesPerSecPerGiB    int64
	MinReadPercent       int32
	MaxReadPercent       int32
	MinAvgSizeBytes      int32
	MaxAvgSizeBytes      int32
}

// updateServicePlanData updates the ServicePlanData table on first connection
func (c *Component) updateServicePlanData(ctx context.Context) error {
	// fetch service plans
	lParams := service_plan.NewServicePlanListParams()
	lRes, err := c.oCrud.ServicePlanList(ctx, lParams)
	if err != nil {
		return err
	}
	// prepare an upsert statement using a versioned stored procedure
	var stmt *sql.Stmt
	ver := c.TableVersion("ServicePlanData")
	switch ver {
	case 1:
		stmt, err = c.SafePrepare(ctx, "SELECT ServicePlanDataUpsert1($1, $2, $3, $4, $5, $6, $7, $8, $9)")
	default:
		err = fmt.Errorf("unsupported ServicePlanData version %d", ver)
	}
	if err != nil {
		return err
	}
	defer stmt.Close()
	// convert response time values to microseconds (max in db ~ 30min)
	sloResponseTimeUSec := func(plan *models.ServicePlan, sn string) int32 {
		if slo, ok := plan.Slos[sn]; ok {
			if d, err := time.ParseDuration(slo.Value); err == nil {
				return int32(d / time.Microsecond)
			}
		}
		return 0
	}
	// upsert service plan data
	for _, plan := range lRes.Payload {
		rt := sloResponseTimeUSec(plan, SloResponseTimeAverage)
		rtMax := sloResponseTimeUSec(plan, SloResponseTimeMaximum)
		iopsPerGiB := int32(swag.Int64Value(plan.ProvisioningUnit.IOPS))
		bpsPerGiB := swag.Int64Value(plan.ProvisioningUnit.Throughput)
		minReadPct := swag.Int32Value(plan.IoProfile.ReadWriteMix.MinReadPercent)
		maxReadPct := swag.Int32Value(plan.IoProfile.ReadWriteMix.MaxReadPercent)
		minAvgSizeBytes := swag.Int32Value(plan.IoProfile.IoPattern.MinSizeBytesAvg)
		maxAvgSizeBytes := swag.Int32Value(plan.IoProfile.IoPattern.MaxSizeBytesAvg)
		if ver == 1 {
			_, err := stmt.ExecContext(ctx, string(plan.Meta.ID), rt, rtMax, iopsPerGiB, bpsPerGiB, minReadPct, maxReadPct, minAvgSizeBytes, maxAvgSizeBytes)
			if err != nil {
				err = fmt.Errorf("%s (%s)", err.Error(), c.pgDB.SQLCode(err))
				return err
			}
		}
		spd := &ServicePlanData{
			ResponseTimeMeanUsec: rt,
			ResponseTimeMaxUsec:  rtMax,
			IOPsPerGiB:           iopsPerGiB,
			BytesPerSecPerGiB:    bpsPerGiB,
			MinReadPercent:       minReadPct,
			MaxReadPercent:       maxReadPct,
			MinAvgSizeBytes:      minAvgSizeBytes,
			MaxAvgSizeBytes:      maxAvgSizeBytes,
		}
		c.cacheServicePlan(string(plan.Meta.ID), spd)
	}
	c.servicePlanDataUpdated = true
	return nil
}

func (c *Component) cacheServicePlan(spID string, spd *ServicePlanData) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.servicePlanCache == nil {
		c.servicePlanCache = make(map[string]*ServicePlanData)
	}
	c.servicePlanCache[spID] = spd
}

func (c *Component) lookupServicePlan(spID string) *ServicePlanData {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.servicePlanCache != nil {
		if sp, ok := c.servicePlanCache[spID]; ok {
			return sp
		}
	}
	return nil
}
