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


package crude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	nuvo "github.com/Nuvoloso/kontroller/pkg/autogen/client"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/ws"
	"github.com/go-openapi/strfmt"
	"github.com/gorilla/websocket"
	"github.com/op/go-logging"
)

// WatcherCallbackType describes the purpose of the callback
type WatcherCallbackType int

// WatcherCallbackType values
const (
	WatcherEvent WatcherCallbackType = iota
	WatcherQuitting
)

// Watcher is an interface through which event notification is made to a client.
type Watcher interface {
	// CrudeNotify delivers events to the client.
	// A returned error terminates the Watcher (with a WatcherQuitting callback).
	CrudeNotify(WatcherCallbackType, *CrudEvent) error
}

// ScopeMap is used to contain scope properties
type ScopeMap map[string]string

// CrudEvent describes a successful CRUD operation.
type CrudEvent struct {
	Timestamp          time.Time
	Ordinal            int64
	Method             string
	TrimmedURI         string
	ScopeS             string // needs assembly
	Scope              ScopeMap
	AccessControlScope interface{}
	FromUpstream       bool
}

// Validate is used to check a user constructed event
func (ce *CrudEvent) Validate() error {
	if !util.Contains([]string{"POST", "PATCH", "DELETE"}, ce.Method) || ce.TrimmedURI == "" || !strings.HasPrefix(ce.TrimmedURI, "/") {
		return fmt.Errorf("invalid or missing fields")
	}
	return nil
}

func (ce *CrudEvent) cook() *CrudEvent {
	if len(ce.Scope) > 0 {
		sKeys := util.SortedStringKeys(ce.Scope)
		ss := make([]string, len(ce.Scope))
		for i, k := range sKeys {
			ss[i] = fmt.Sprintf("%s:%s", k, ce.Scope[k])
		}
		ce.ScopeS = strings.Join(ss, " ")
	}
	return ce
}

// FromModel imports a model object
func (ce *CrudEvent) FromModel(mObj *models.CrudEvent) {
	ce.Timestamp = time.Time(mObj.Timestamp)
	ce.Ordinal = mObj.Ordinal
	ce.Method = mObj.Method
	ce.TrimmedURI = mObj.TrimmedURI
	ce.Scope = mObj.Scope
	ce.AccessControlScope = nil
	ce.cook()
}

// ToModel converts to the model data type
func (ce *CrudEvent) ToModel() *models.CrudEvent {
	return &models.CrudEvent{
		Timestamp:  strfmt.DateTime(ce.Timestamp),
		Ordinal:    ce.Ordinal,
		Method:     ce.Method,
		TrimmedURI: ce.TrimmedURI,
		Scope:      ce.Scope,
		// AccessControlScope is not in the model object
	}
}

const (
	// ScopeMetaID is the name of the scope map key for object id. Only for POST.
	ScopeMetaID = "meta.id"

	// ScopeMetaVersion is the name of the scope map key for object version.
	ScopeMetaVersion = "meta.version"
)

var uriIDPattern = "^.*/([^\\?]+)\\??" // not for POST, which has no id
var uriIDRe = regexp.MustCompile(uriIDPattern)

// ID returns the object ID from the URI or the meta.id property in the scope map
func (ce *CrudEvent) ID() string {
	if ce.Method != "POST" {
		m := uriIDRe.FindStringSubmatchIndex(ce.TrimmedURI)
		if len(m) == 4 {
			return ce.TrimmedURI[m[2]:m[3]]
		}
	} else if id, ok := ce.Scope[ScopeMetaID]; ok {
		return id
	}
	return ""
}

// Ops contains the operational interfaces
type Ops interface {
	CollectorMiddleware(next http.Handler) http.Handler
	SetScope(r *http.Request, props ScopeMap, accessScope interface{})
	RegisterHandlers(api *operations.NuvolosoAPI, accessManager AccessControl)
	Watch(mObj *models.CrudWatcherCreateArgs, client Watcher) (string, error)
	TerminateWatcher(id string)
	TerminateAllWatchers()
	MonitorUpstream(api mgmtclient.API, apiArgs *mgmtclient.APIArgs, mObj *models.CrudWatcherCreateArgs) error
	GetStats() Stats
	InjectEvent(ce *CrudEvent) error
}

// AccessControl is the interface to support access control on watchers
type AccessControl interface {
	auth.AccessControl
	EventAllowed(auth.Subject, *CrudEvent) bool
}

// dispatcher processes events internally
type dispatcher interface {
	dispatchEvent(ev *CrudEvent)
}

// ManagerArgs contains arguments to create a Manager
type ManagerArgs struct {
	WSKeepAlive bool `long:"keep-websockets-alive" description:"Periodically ping web socket clients. Required if a proxy is used."`
	Log         *logging.Logger
}

// Manager represents the event management subsystem.
type Manager struct {
	ManagerArgs
	mux       sync.Mutex
	cond      *sync.Cond
	oUpgrader websocket.Upgrader
	dispatcher
	upgrader      ws.Upgrader
	accessManager AccessControl
	watchers      []*watcher
	ceCounter     int64
	upMon         *upstreamMonitor
	upMonStop     bool
	// stat counters
	activatedCount     int
	localEventCount    int
	upstreamEventCount int
}

// Stats returns statistics
type Stats struct {
	NumWatchersActivated int
	NumActiveWatchers    int
	NumLocalEvents       int
	NumUpstreamEvents    int
	UpstreamMonitor      UpstreamMonitorStats
}

func (s Stats) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Watcher constants
const (
	WatcherActivationLimit = time.Duration(30 * time.Second)
	WatcherURLRelPath      = "/watchers"
	WatcherURLPathPrefix   = nuvo.DefaultBasePath + WatcherURLRelPath
)

// NewManager returns a new Manager.
func NewManager(args *ManagerArgs) Ops {
	m := &Manager{}
	m.ManagerArgs = *args
	m.dispatcher = m // self-reference
	m.watchers = make([]*watcher, 0)
	m.oUpgrader.ReadBufferSize = 1024
	m.oUpgrader.WriteBufferSize = 1024
	m.upgrader = ws.WrapUpgrader(&m.oUpgrader)
	m.cond = sync.NewCond(&m.mux)
	m.ceCounter = time.Now().Unix()
	return m
}

// ClientURL constructs the WebSocket client URL for a watcher
func ClientURL(args *mgmtclient.APIArgs, watcherID string) string {
	proto := "ws"
	hp := fmt.Sprintf("%s:%d", args.Host, args.Port)
	if args.SocketPath != "" {
		hp = "localhost"
	} else if (args.TLSCertificate != "" && args.TLSCertificateKey != "") || args.ForceTLS {
		proto = "wss"
	}
	return fmt.Sprintf("%s://%s%s/%s", proto, hp, WatcherURLPathPrefix, watcherID)
}

// activateWatcher activates a watcher if not already terminated (shutdown race)
// and not already activated (single-use)
func (m *Manager) activateWatcher(id string) *watcher {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.purgeExpiredInLock()
	for _, w := range m.watchers {
		if w.id == id && !w.terminate && !w.active {
			w.active = true
			m.activatedCount++
			return w
		}
	}
	return nil
}

func (m *Manager) removeWatcher(w *watcher) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.Log.Debugf("Removing watcher %s", w.id)
	m.removeWatcherInLock(w)
}

func (m *Manager) removeWatcherInLock(w *watcher) {
	w.active = false
	w.terminate = true
	pos := -1
	for i, wp := range m.watchers {
		if w == wp {
			pos = i
			break
		}
	}
	if pos != -1 {
		copy(m.watchers[pos:], m.watchers[pos+1:])
		m.watchers[len(m.watchers)-1] = nil
		m.watchers = m.watchers[:len(m.watchers)-1]
	}
	for i := 0; i < len(w.ceQ); i++ {
		w.ceQ[i] = nil
	}
	m.cond.Signal()
}

// purgeExpired must be called from within the mutex to purge expired inactive watchers
func (m *Manager) purgeExpiredInLock() {
	now := time.Now()
	expired := make([]*watcher, 0, 4)
	for _, w := range m.watchers {
		if !w.active && !w.terminate && now.After(w.activateByTime) {
			m.Log.Debugf("Watcher %s expired", w.id)
			expired = append(expired, w)
		}
	}
	for _, w := range expired {
		m.removeWatcherInLock(w)
	}
}

// dispatchEvent dispatches an event to consumers. It should be thread safe and not block because it
// is invoked from the collector middleware.
// It assigns a timestamp and an ordinal if not from upstream. Note that this assumes (but doesn't enforce) one collection source.
func (m *Manager) dispatchEvent(ce *CrudEvent) {
	m.mux.Lock()
	defer m.mux.Unlock()
	if !ce.FromUpstream {
		ce.Timestamp = time.Now()
		ce.Ordinal = m.ceCounter
		m.ceCounter++
		m.localEventCount++
	} else {
		m.upstreamEventCount++
	}
	m.Log.Debugf("EVENT %d %v [%s] [%s] [%s]", ce.Ordinal, ce.FromUpstream, ce.Method, ce.TrimmedURI, ce.ScopeS)
	for _, w := range m.watchers {
		if w.active && w.match(ce) {
			w.mux.Lock()
			m.Log.Debugf("EVENT %d ⇒ %s", ce.Ordinal, w.id)
			w.ceQ = append(w.ceQ, ce)
			w.cond.Signal()
			w.mux.Unlock()
		}
	}
}

// InjectEvent is used to explicitly insert an event
func (m *Manager) InjectEvent(ce *CrudEvent) error {
	ce.FromUpstream = false
	if err := ce.Validate(); err != nil {
		return err
	}
	ce.cook()
	m.dispatchEvent(ce)
	return nil
}

// MonitorUpstream collects events from an upstream source and dispatches them locally.
// It will collect until all watchers are terminated.
// It will retry if the upstream connection breaks.
func (m *Manager) MonitorUpstream(api mgmtclient.API, apiArgs *mgmtclient.APIArgs, mObj *models.CrudWatcherCreateArgs) error {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.upMon != nil {
		return fmt.Errorf("already configured")
	}
	if api == nil || apiArgs == nil || mObj == nil {
		return fmt.Errorf("invalid arguments")
	}
	// validate the args locally
	w := &watcher{}
	w.init(m, nil)
	if err := w.parseArgs(mObj); err != nil {
		return err
	}
	// create a web socket dialer
	dialer, err := mgmtclient.NewWebSocketDialer(apiArgs)
	if err != nil {
		m.Log.Errorf("Unable to create WebSocket dialer: %s", err.Error())
		return err
	}
	// launch the monitor in a goroutine
	m.upMon = &upstreamMonitor{
		m:       m,
		api:     api,
		dialer:  dialer,
		apiArgs: apiArgs,
		mObj:    mObj,
	}
	go util.PanicLogger(m.Log, m.upMon.run)
	return nil
}

// Watch registers a client callback for CrudEvents
func (m *Manager) Watch(mObj *models.CrudWatcherCreateArgs, client Watcher) (string, error) {
	if mObj == nil || client == nil {
		return "", fmt.Errorf("invalid arguments")
	}
	id, err := m.newWatcher(mObj, client)
	if err == nil {
		w := m.activateWatcher(id)
		go w.deliverEvents()
	}
	return id, err
}

// TerminateWatcher schedules the future termination of a Watcher (if it exists)
func (m *Manager) TerminateWatcher(id string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	for _, w := range m.watchers {
		if w.id == id {
			if w.active && !w.terminate {
				w.mustTerminate()
				break
			}
		}
	}
}

// TerminateAllWatchers schedules the termination of all Watchers and the upstream monitor (if configured)
// It blocks until termination.
func (m *Manager) TerminateAllWatchers() {
	m.mux.Lock()
	defer m.mux.Unlock()
	if m.upMon != nil {
		m.upMon.stopInLock()
	}
	m.Log.Infof("Terminating all watchers")
	inactive := make([]*watcher, 0, 4)
	for _, w := range m.watchers {
		if !w.active && !w.terminate {
			inactive = append(inactive, w)
		}
	}
	for _, w := range inactive {
		m.removeWatcherInLock(w)
	}
	for _, w := range m.watchers {
		if !w.terminate {
			w.mustTerminate()
		}
	}
	for len(m.watchers) > 0 {
		m.Log.Debugf("Waiting for %d watchers to terminate", len(m.watchers))
		m.cond.Wait()
	}
	for m.upMon != nil {
		m.Log.Debugf("Waiting for upstream monitor to terminate")
		m.cond.Wait()
	}
	m.Log.Infof("All watchers terminated")
}

// GetStats returns statistical information
func (m *Manager) GetStats() Stats {
	m.mux.Lock()
	defer m.mux.Unlock()
	numActive := 0
	for _, w := range m.watchers {
		if w.active {
			numActive++
		}
	}
	stats := Stats{
		NumWatchersActivated: m.activatedCount,
		NumActiveWatchers:    numActive,
		NumLocalEvents:       m.localEventCount,
		NumUpstreamEvents:    m.upstreamEventCount,
	}
	if m.upMon != nil {
		stats.UpstreamMonitor = m.upMon.getStats()
	}
	return stats
}
