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


package util

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/strfmt"
	logging "github.com/op/go-logging"
)

// Controller is the abstraction of a NuvoService
type Controller interface {
	SetServiceIP(sIP string)
	GetServiceIP() string
	SetServiceLocator(sLocator string)
	GetServiceLocator() string
	SetState(State) error
	GetState() State
	StateString(State) string
	Message(format string, args ...interface{})
	ReplaceMessage(pattern string, format string, args ...interface{})
	ModelObj() *models.NuvoService
	Import(*models.NuvoService) error
	GetServiceAttribute(name string) *models.ValueType
	SetServiceAttribute(name string, value models.ValueType)
	RemoveServiceAttribute(name string)
}

// ServiceArgs are the properties required to create a Service object
type ServiceArgs struct {
	ServiceType         string
	ServiceVersion      string
	ServiceIP           string // can also be set after creation
	ServiceLocator      string // can also be set after creation
	HeartbeatPeriodSecs int64
	ServiceAttributes   map[string]models.ValueType // can be modified after creation
	Log                 *logging.Logger
	MaxMessages         int
}

// Service is an implementation of Controller
type Service struct {
	ServiceArgs
	state    State
	messages []*models.TimestampedString
	mux      sync.Mutex
}

var _ = Controller(&Service{})

// State numeric type
type State int

// State numeric constants
const (
	ServiceStopped State = iota
	ServiceStarting
	ServiceReady
	ServiceError
	ServiceStopping
	ServiceNotReady
)

// defaults
const (
	ServiceDefaultMaxMessages = 20
)

// NewService returns an implementation of Controller
func NewService(args *ServiceArgs) Controller {
	svc := &Service{
		state:       ServiceStopped,
		ServiceArgs: *args,
	}
	if svc.MaxMessages <= 0 {
		svc.MaxMessages = ServiceDefaultMaxMessages
	}
	svc.messages = make([]*models.TimestampedString, 0, svc.MaxMessages+1)
	if svc.ServiceArgs.ServiceAttributes == nil {
		svc.ServiceArgs.ServiceAttributes = make(map[string]models.ValueType)
	}
	return svc
}

// SetServiceIP sets the service IP
func (svc *Service) SetServiceIP(sIP string) {
	svc.ServiceIP = sIP
}

// GetServiceIP returns the service IP
func (svc *Service) GetServiceIP() string {
	return svc.ServiceIP
}

// SetServiceLocator sets the service Locator
func (svc *Service) SetServiceLocator(sLocator string) {
	svc.ServiceLocator = sLocator
}

// GetServiceLocator returns the service Locator
func (svc *Service) GetServiceLocator() string {
	return svc.ServiceLocator
}

// GetServiceAttribute returns a service attribute value or nil
func (svc *Service) GetServiceAttribute(name string) *models.ValueType {
	svc.mux.Lock()
	defer svc.mux.Unlock()
	if vt, ok := svc.ServiceArgs.ServiceAttributes[name]; ok {
		return &vt
	}
	return nil
}

// SetServiceAttribute sets a service attribute
func (svc *Service) SetServiceAttribute(name string, value models.ValueType) {
	svc.mux.Lock()
	defer svc.mux.Unlock()
	svc.ServiceArgs.ServiceAttributes[name] = value
}

// RemoveServiceAttribute sets a service attribute
func (svc *Service) RemoveServiceAttribute(name string) {
	svc.mux.Lock()
	defer svc.mux.Unlock()
	delete(svc.ServiceArgs.ServiceAttributes, name)
}

// StateString provides a mapping of the State to string
func (svc *Service) StateString(s State) string {
	switch s {
	case ServiceStarting:
		return "STARTING"
	case ServiceStopped:
		return "STOPPED"
	case ServiceNotReady:
		return "NOT_READY"
	case ServiceReady:
		return common.ServiceStateReady
	case ServiceError:
		return "ERROR"
	case ServiceStopping:
		return "STOPPING"
	default:
		return common.ServiceStateUnknown
	}
}

// SetState changes the externally visible state of the service
func (svc *Service) SetState(newState State) error {
	nss := svc.StateString(newState)
	if nss == "UNKNOWN" {
		err := fmt.Errorf("Invalid state change request: \"%d\" set", newState)
		svc.Log.Warning("Error:", err.Error())
		return err
	}
	svc.mux.Lock()
	defer svc.mux.Unlock()
	if newState != svc.state {
		oss := svc.StateString(svc.state)
		msg := svc.m("State change: %s ⇒ %s", oss, nss)
		svc.Log.Info(msg)
		svc.state = newState
	}
	return nil
}

// GetState returns the state
func (svc *Service) GetState() State {
	return svc.state
}

// Message adds a message, possibly eliminating older messages
func (svc *Service) Message(format string, args ...interface{}) {
	svc.mux.Lock()
	defer svc.mux.Unlock()
	svc.m(format, args...)
}

// ReplaceMessage removes messages that match a regular expression and adds the specified
// message at the end.
// Additional older messages may also be eliminated based on max size.
func (svc *Service) ReplaceMessage(pattern string, format string, args ...interface{}) {
	svc.mux.Lock()
	defer svc.mux.Unlock()
	re := regexp.MustCompile(pattern)
	nm := make([]*models.TimestampedString, 0, len(svc.messages))
	for i := 0; i < len(svc.messages); i++ {
		if re.MatchString(svc.messages[i].Message) {
			continue
		}
		nm = append(nm, svc.messages[i])
	}
	svc.messages = nm
	svc.m(format, args...)
}

// m is the unprotected variant of Message and also returns the string
func (svc *Service) m(format string, args ...interface{}) string {
	svc.messages = NewMsgList(svc.messages).Insert(format, args...).ToModel()
	msg := svc.messages[len(svc.messages)-1].Message
	svc.adjustMessages(false)
	return msg
}

func (svc *Service) adjustMessages(needToSort bool) {
	if needToSort {
		sort.Slice(svc.messages, func(i, j int) bool {
			ti := time.Time(svc.messages[i].Time)
			tj := time.Time(svc.messages[j].Time)
			return ti.Before(tj)
		})
	}
	// shift older messages, avoid duplicate pointer references
	delta := len(svc.messages) - svc.MaxMessages
	if delta > 0 {
		for i, j := 0, delta; j < len(svc.messages); i, j = (i + 1), (j + 1) {
			svc.messages[i] = svc.messages[j]
		}
		for j := len(svc.messages) - delta; j < len(svc.messages); j++ {
			svc.messages[j] = nil // clear previous pointers
		}
		svc.messages = svc.messages[:len(svc.messages)-delta] // truncate length
	}
}

// ModelObj returns the model object
func (svc *Service) ModelObj() *models.NuvoService {
	svc.mux.Lock()
	defer svc.mux.Unlock()
	messages := make([]*models.TimestampedString, len(svc.messages))
	copy(messages, svc.messages)
	return &models.NuvoService{
		NuvoServiceAllOf0: models.NuvoServiceAllOf0{
			Messages:          messages,
			ServiceAttributes: svc.ServiceAttributes,
			ServiceType:       svc.ServiceType,
			ServiceVersion:    svc.ServiceVersion,
			ServiceIP:         svc.ServiceIP,
			ServiceLocator:    svc.ServiceLocator,
		},
		ServiceState: models.ServiceState{
			HeartbeatPeriodSecs: svc.HeartbeatPeriodSecs,
			HeartbeatTime:       strfmt.DateTime(time.Now()),
			State:               svc.StateString(svc.state),
		},
	}
}

// Import imports previous transferrable state from a model object.
// The serviceType has to match.
// Only previous messages are considered transferrable.
func (svc *Service) Import(o *models.NuvoService) error {
	if svc.ServiceType != o.ServiceType {
		return fmt.Errorf("service type mismatch on import")
	}
	if o.Messages != nil {
		svc.mux.Lock()
		defer svc.mux.Unlock()
		svc.messages = append(svc.messages, o.Messages...)
		svc.adjustMessages(true)
	}
	return nil
}
