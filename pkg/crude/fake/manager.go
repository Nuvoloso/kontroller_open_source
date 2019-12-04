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
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
)

// Manager is a fake CRUD event manager
type Manager struct {
	// GetStats
	RetGS crude.Stats

	// InjectEvent
	InjectFirstEvent *crude.CrudEvent
	InIEev           *crude.CrudEvent
	RetIEerr         error
	CalledIEnum      int

	// MonitorUpstream
	CalledMU bool
	RetMUErr error

	// RegisterHandlers
	CalledRH bool

	// SetScope
	InSSProps crude.ScopeMap
	InACScope interface{}

	// TerminateAllWatchers
	CalledTAW bool

	// Watch
	InWObj  *models.CrudWatcherCreateArgs
	RetWID  string
	RetWErr error
}

// NewFakeEventManager creates a fake CRUD event manager
func NewFakeEventManager() *Manager {
	m := &Manager{}
	return m
}

var _ = crude.Ops(&Manager{})

// CollectorMiddleware fakes the interface of the same name
func (m *Manager) CollectorMiddleware(next http.Handler) http.Handler {
	return nil
}

// SetScope fakes the interface of the same name
func (m *Manager) SetScope(r *http.Request, props crude.ScopeMap, accessScope interface{}) {
	m.InSSProps = props
	m.InACScope = accessScope
}

// RegisterHandlers fakes the interface of the same name
func (m *Manager) RegisterHandlers(api *operations.NuvolosoAPI, _ crude.AccessControl) {
	m.CalledRH = true
}

// Watch fakes the interface of the same name
func (m *Manager) Watch(mObj *models.CrudWatcherCreateArgs, client crude.Watcher) (string, error) {
	m.InWObj = mObj
	return m.RetWID, m.RetWErr
}

// TerminateWatcher fakes the interface of the same name
func (m *Manager) TerminateWatcher(id string) {
}

// TerminateAllWatchers fakes the interface of the same name
func (m *Manager) TerminateAllWatchers() {
	m.CalledTAW = true
}

// MonitorUpstream fakes the interface of the same name
func (m *Manager) MonitorUpstream(api mgmtclient.API, apiArgs *mgmtclient.APIArgs, mObj *models.CrudWatcherCreateArgs) error {
	m.CalledMU = true
	return m.RetMUErr
}

// GetStats fakes the interface of the same name
func (m *Manager) GetStats() crude.Stats {
	return m.RetGS
}

// InjectEvent fakes the interface of the same name
func (m *Manager) InjectEvent(ce *crude.CrudEvent) error {
	if m.InjectFirstEvent == nil {
		m.InjectFirstEvent = ce
	}
	m.InIEev = ce
	m.CalledIEnum++
	return m.RetIEerr
}
