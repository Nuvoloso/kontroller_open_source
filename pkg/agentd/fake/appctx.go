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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
)

// AppServant implements fakes the operations of agentd.AppServant
type AppServant struct {
	// AddStorage
	InAddStorageS *agentd.Storage

	// GetServicePlan
	InGSPid   string
	RetGSPObj *models.ServicePlan
	RetGSPErr error

	// FatalError
	CountFatalError int

	// AddLUN
	InAddLUNObj *agentd.LUN

	// FindLUN
	InFLvsID   string
	InFLSnapID string
	RetFLObj   *agentd.LUN

	// IsReady
	RetIsReady bool

	// RemoveLUN
	InRemoveLUNvsID       string
	InRemoveLUNvsIDSnapID string

	// RemoveStorage
	InRemoveStorageID string
}

var _ = agentd.AppServant(&AppServant{})

// AddLUN fakes its namesake interface method
func (as *AppServant) AddLUN(lun *agentd.LUN) {
	as.InAddLUNObj = lun
}

// AddStorage fakes its namesake interface method
func (as *AppServant) AddStorage(storage *agentd.Storage) {
	as.InAddStorageS = storage
}

// FatalError fakes its namesake interface method
func (as *AppServant) FatalError(error) {
	as.CountFatalError++
}

// FindLUN fakes its namesake interface method
func (as *AppServant) FindLUN(vsID string, snapID string) *agentd.LUN {
	as.InFLvsID = vsID
	as.InFLSnapID = snapID
	return as.RetFLObj
}

// FindStorage fakes its namesake interface method
func (as *AppServant) FindStorage(storageID string) *agentd.Storage {
	return nil
}

// GetCentraldAPIArgs fakes its namesake interface method
func (as *AppServant) GetCentraldAPIArgs() *mgmtclient.APIArgs {
	return nil
}

// GetClusterdAPI fakes its namesake interface method
func (as *AppServant) GetClusterdAPI() (mgmtclient.API, error) {
	return nil, nil
}

// GetClusterdAPIArgs fakes its namesake interface method
func (as *AppServant) GetClusterdAPIArgs() *mgmtclient.APIArgs {
	return nil
}

// GetLUNs fakes its namesake interface method
func (as *AppServant) GetLUNs() []*agentd.LUN {
	return nil
}

// GetServicePlan fakes its namesake interface method
func (as *AppServant) GetServicePlan(ctx context.Context, spID string) (*models.ServicePlan, error) {
	as.InGSPid = spID
	return as.RetGSPObj, as.RetGSPErr
}

// GetStorage fakes its namesake interface method
func (as *AppServant) GetStorage() []*agentd.Storage {
	return nil
}

// InitializeCSPClient fakes its namesake interface method
func (as *AppServant) InitializeCSPClient(dObj *models.CSPDomain) error {
	return nil
}

// RemoveLUN fakes its namesake interface method
func (as *AppServant) RemoveLUN(vsID string, snapID string) {
	as.InRemoveLUNvsID, as.InRemoveLUNvsIDSnapID = vsID, snapID
}

// RemoveStorage fakes its namesake interface method
func (as *AppServant) RemoveStorage(storageID string) {
	as.InRemoveStorageID = storageID
}

// InitializeNuvo fakes its namesake interface method
func (as *AppServant) InitializeNuvo(ctx context.Context) error {
	return nil
}

// IsReady fakes its namesake interface method
func (as *AppServant) IsReady() bool {
	return as.RetIsReady
}
