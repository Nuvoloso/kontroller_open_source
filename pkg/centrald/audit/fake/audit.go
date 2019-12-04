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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
)

// Args records the arguments passed to an AppAudit function
type Args struct {
	AI      auth.Subject
	Parent  int32
	Action  centrald.AuditAction
	ObjID   models.ObjID
	Name    models.ObjName
	RefID   models.ObjIDMutable
	Err     bool
	Message string
}

// AuditLog fakes its namesake
type AuditLog struct {
	ReadyRet            error
	RegisterHandlersAPI *operations.NuvolosoAPI

	Annotations []*Args
	Events      []*Args
	Posts       []*Args

	ExpireCtx context.Context
	ExpireRet error
}

var _ = centrald.AppAudit(&AuditLog{})

// Ready implements the AppAudit interface
func (fa *AuditLog) Ready() error {
	return fa.ReadyRet
}

// RegisterHandlers implements the AppAudit interface
func (fa *AuditLog) RegisterHandlers(api *operations.NuvolosoAPI) {
	fa.RegisterHandlersAPI = api
	fa.Annotations = make([]*Args, 0, 1)
	fa.Events = make([]*Args, 0, 1)
	fa.Posts = make([]*Args, 0, 1)
}

// Annotation implements the AppAudit interface
func (fa *AuditLog) Annotation(ctx context.Context, ai auth.Subject, parent int32, action centrald.AuditAction, oid models.ObjID, name models.ObjName, err bool, msg string) {
	args := &Args{AI: ai, Parent: parent, Action: action, ObjID: oid, Name: name, Err: err, Message: msg}
	fa.Annotations = append(fa.Annotations, args)
}

// Event implements the AppAudit interface
func (fa *AuditLog) Event(ctx context.Context, ai auth.Subject, action centrald.AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, msg string) {
	args := &Args{AI: ai, Action: action, ObjID: oid, Name: name, RefID: refID, Err: err, Message: msg}
	fa.Events = append(fa.Events, args)
}

// Post implements the AppAudit interface
func (fa *AuditLog) Post(ctx context.Context, ai auth.Subject, action centrald.AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, msg string) {
	args := &Args{AI: ai, Action: action, ObjID: oid, Name: name, RefID: refID, Err: err, Message: msg}
	fa.Posts = append(fa.Posts, args)
}

// Expire implements the AppAudit interface
func (fa *AuditLog) Expire(ctx context.Context, baseTime time.Time) error {
	fa.ExpireCtx = ctx
	return fa.ExpireRet
}
