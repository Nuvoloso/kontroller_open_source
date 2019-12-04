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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
)

// AppServant implements fakes the operations of clusterd.AppServant
type AppServant struct {
	InGSPid   string
	RetGSPObj *models.ServicePlan
	RetGSPErr error

	CountFatalError int
}

var _ = clusterd.AppServant(&AppServant{})

// FatalError fakes its namesake interface method
func (as *AppServant) FatalError(error) {
	as.CountFatalError++
}

// GetAPIArgs fakes its namesake interface method
func (as *AppServant) GetAPIArgs() *mgmtclient.APIArgs {
	return nil
}

// InitializeCSPClient fakes its namesake interface method
func (as *AppServant) InitializeCSPClient(dObj *models.CSPDomain) error {
	return nil
}

// GetServicePlan fakes its namesake interface method
func (as *AppServant) GetServicePlan(ctx context.Context, spID string) (*models.ServicePlan, error) {
	as.InGSPid = spID
	return as.RetGSPObj, as.RetGSPErr
}
