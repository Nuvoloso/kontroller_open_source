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


package audit

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// LogRecord contains audit log data to be inserted into the database
type LogRecord struct {
	Timestamp       time.Time
	ParentNum       int32
	Classification  string
	ObjectType      string
	ObjectID        string
	ObjectName      string // empty for objects with no name-like field
	RefObjectID     string
	TenantAccountID string
	AccountID       string
	AccountName     string
	UserID          string
	AuthIdentifier  string
	Action          string
	Error           bool
	Message         string
}

// interface for testing
type logInserter interface {
	insert(context.Context, *LogRecord) (int32, error)
}

// logHandler manages log records
type logHandler struct {
	c   *Component
	i   logInserter
	mux sync.Mutex
}

func (h *logHandler) Init(c *Component) {
	h.c = c
	h.i = h // self-reference
}

func (h *logHandler) Start() {
}

func (h *logHandler) Stop() {
}

func (h *logHandler) prepareStmt(ctx context.Context) (*sql.Stmt, error) {
	var stmt *sql.Stmt
	var err error
	switch ver := h.c.TableVersion("AuditLog"); ver {
	case 1:
		stmt, err = h.c.SafePrepare(ctx, "SELECT AuditLogInsert1($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)")
	default:
		err = fmt.Errorf("unsupported AuditLog version %d", ver)
	}
	return stmt, err
}

func (h *logHandler) insert(ctx context.Context, m *LogRecord) (int32, error) {
	err := h.c.Ready()
	if err != nil {
		return 0, err
	}
	stmt := h.c.StmtCacheGet("AuditLog")
	if stmt == nil {
		if stmt, err = h.prepareStmt(ctx); err != nil {
			return 0, err
		}
		h.c.StmtCacheSet("AuditLog", stmt)
	}
	switch ver := h.c.TableVersion("AuditLog"); ver {
	case 1:
		row := stmt.QueryRowContext(ctx, m.Timestamp, m.ParentNum, m.Classification, m.ObjectType, m.ObjectID, m.ObjectName, m.RefObjectID, m.TenantAccountID, m.AccountID, m.AccountName, m.UserID, m.AuthIdentifier, m.Action, m.Error, m.Message)
		var seq int32
		if err = row.Scan(&seq); err == nil {
			h.c.Log.Debugf("Inserted AuditLog record [%d]", seq)
			return seq, nil
		}
	}
	if h.c.pgDB.ErrDesc(err) == pgdb.ErrConnectionError {
		h.c.Notify() // awaken the main loop to ping the DB
	}
	h.c.Log.Errorf("Failed to insert AuditLog: %s", err.Error())
	return 0, h.c.Errorf("%s", err.Error())
}

// expire deletes outdated records from audit log according to the settings
func (h *logHandler) expire(ctx context.Context, baseTime time.Time) error {
	switch ver := h.c.TableVersion("AuditLog"); ver {
	case 1:
		// supported. TBD reorganize the code to have versioned list support
	default:
		h.c.Log.Errorf("Unsupported AuditLog version %d", ver)
		return centrald.ErrorDbError
	}
	h.c.mux.Lock()
	defer h.c.mux.Unlock()
	if h.c.db == nil {
		return centrald.ErrorDbError
	}
	qBase := "DELETE FROM AuditLog"
	deleteParams := map[string]string{
		com.AnnotationClass: strfmt.DateTime(baseTime.Add(-h.c.AuditAnnotationRetentionPeriod)).String(),
		com.AuditClass:      strfmt.DateTime(baseTime.Add(-h.c.AuditRecordRetentionPeriod)).String(),
		com.EventClass:      strfmt.DateTime(baseTime.Add(-h.c.AuditEventRetentionPeriod)).String(),
	}
	for _, class := range util.SortedStringKeys(deleteParams) {
		where, args := []string{}, []interface{}{}
		where = append(where, fmt.Sprintf("Classification = $%d", 1))
		args = append(args, class)
		where = append(where, fmt.Sprintf("TimeStamp < $%d", 2))
		args = append(args, deleteParams[class])

		q := qBase + " WHERE " + strings.Join(where, " AND ")
		h.c.Log.Debugf("Executing AuditLog query %s with args %v", q, args)
		res, err := h.c.db.ExecContext(ctx, q, args...)
		if err != nil {
			h.c.Log.Errorf("AuditLog delete %s records query error: %s", class, err.Error())
			return centrald.ErrorDbError
		}
		numRowsDeleted, err := res.RowsAffected()
		if err != nil {
			h.c.Log.Errorf("AuditLog error to process delete query results: %s", err.Error())
			return centrald.ErrorDbError
		}
		h.c.Log.Debugf("AuditLog successfully deleted %d %s records", numRowsDeleted, class)
	}
	return nil
}

type auditAction struct {
	oType   string
	oAction string
}

var actionMap = map[centrald.AuditAction]auditAction{
	centrald.AccountCreateAction: {"account", "create"},
	centrald.AccountDeleteAction: {"account", "delete"},
	centrald.AccountUpdateAction: {"account", "update"},

	centrald.ApplicationGroupCreateAction: {"applicationgroup", "create"},
	centrald.ApplicationGroupDeleteAction: {"applicationgroup", "delete"},
	centrald.ApplicationGroupUpdateAction: {"applicationgroup", "update"},

	centrald.ClusterCreateAction: {"cluster", "create"},
	centrald.ClusterDeleteAction: {"cluster", "delete"},
	centrald.ClusterUpdateAction: {"cluster", "update"},

	centrald.ConsistencyGroupCreateAction: {"consistencygroup", "create"},
	centrald.ConsistencyGroupDeleteAction: {"consistencygroup", "delete"},
	centrald.ConsistencyGroupUpdateAction: {"consistencygroup", "update"},

	centrald.CspCredentialCreateAction: {"cspcredential", "create"},
	centrald.CspCredentialDeleteAction: {"cspcredential", "delete"},
	centrald.CspCredentialUpdateAction: {"cspcredential", "update"},

	centrald.CspDomainCreateAction: {"cspdomain", "create"},
	centrald.CspDomainDeleteAction: {"cspdomain", "delete"},
	centrald.CspDomainUpdateAction: {"cspdomain", "update"},

	centrald.NodeCreateAction: {"node", "create"},
	centrald.NodeDeleteAction: {"node", "delete"},
	centrald.NodeUpdateAction: {"node", "update"},

	centrald.ProtectionDomainClearAction:  {"protectiondomain", "clear"},
	centrald.ProtectionDomainCreateAction: {"protectiondomain", "create"},
	centrald.ProtectionDomainDeleteAction: {"protectiondomain", "delete"},
	centrald.ProtectionDomainSetAction:    {"protectiondomain", "set"},
	centrald.ProtectionDomainUpdateAction: {"protectiondomain", "update"},

	centrald.ServicePlanAllocationCreateAction: {"serviceplanallocation", "create"},
	centrald.ServicePlanAllocationDeleteAction: {"serviceplanallocation", "delete"},
	centrald.ServicePlanAllocationUpdateAction: {"serviceplanallocation", "update"},

	centrald.ServicePlanCloneAction:  {"serviceplan", "clone"},
	centrald.ServicePlanDeleteAction: {"serviceplan", "delete"},
	centrald.ServicePlanUpdateAction: {"serviceplan", "update"},

	centrald.UserCreateAction: {"user", "create"},
	centrald.UserDeleteAction: {"user", "delete"},
	centrald.UserUpdateAction: {"user", "update"},

	centrald.VolumeSeriesCreateAction: {"volumeseries", "create"},
	centrald.VolumeSeriesDeleteAction: {"volumeseries", "delete"},
	centrald.VolumeSeriesUpdateAction: {"volumeseries", "update"},
}

// log creates and inserts a log record
func (h *logHandler) log(ctx context.Context, ai *auth.Info, classification string, parent int32, action centrald.AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, message string) {
	actionStrings := actionMap[action]
	m := &LogRecord{
		Timestamp:       time.Now(),
		ParentNum:       parent,
		Classification:  classification,
		ObjectType:      actionStrings.oType,
		ObjectID:        idOrZeroUUID(string(oid)),
		ObjectName:      string(name),
		RefObjectID:     idOrZeroUUID(string(refID)),
		TenantAccountID: idOrZeroUUID(ai.TenantAccountID),
		AccountID:       idOrZeroUUID(ai.AccountID),
		AccountName:     ai.AccountName,
		UserID:          idOrZeroUUID(ai.UserID),
		AuthIdentifier:  ai.AuthIdentifier,
		Action:          actionStrings.oAction,
		Error:           err,
		Message:         message,
	}
	h.i.insert(ctx, m) // ignore error
}

func (h *logHandler) eCode(err interface{}) int {
	if e, ok := err.(*centrald.Error); ok {
		return e.C
	}
	return http.StatusInternalServerError
}

func (h *logHandler) eError(err interface{}) *models.Error {
	var s string
	if e, ok := err.(error); ok {
		s = e.Error()
	} else {
		s = fmt.Sprintf("%s", err)
	}
	return &models.Error{Code: int32(h.eCode(err)), Message: swag.String(s)}
}

// eMissingMsg produces and logs an extended error message
func (h *logHandler) eMissingMsg(format string, args ...interface{}) *models.Error {
	msg := fmt.Sprintf(centrald.ErrorMissing.M+": "+format, args...)
	h.c.Log.Error(msg)
	return &models.Error{Code: int32(centrald.ErrorMissing.C), Message: swag.String(msg)}
}

func (h *logHandler) auditLogCreate(params ops.AuditLogCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	var ai *auth.Info
	if cnv := ctx.Value(auth.InfoKey{}); cnv != nil {
		ai = cnv.(*auth.Info)
	} else {
		err := centrald.ErrorInternalError
		return ops.NewAuditLogCreateDefault(h.eCode(err)).WithPayload(h.eError(err))
	}
	if err := ai.CapOK(centrald.AuditLogAnnotateCap); err != nil {
		return ops.NewAuditLogCreateDefault(h.eCode(err)).WithPayload(h.eError(err))
	}
	obj := params.Payload
	if obj.Classification != com.AnnotationClass {
		e := h.eMissingMsg("classification must be %s", com.AnnotationClass)
		return ops.NewAuditLogCreateDefault(int(e.Code)).WithPayload(e)
	}
	if ai.AccountID != "" {
		// use auth info when specified (otherwise caller has internal role)
		obj.AccountID = models.ObjIDMutable(ai.AccountID)
		obj.AccountName = ai.AccountName
		obj.AuthIdentifier = ai.AuthIdentifier
		obj.TenantAccountID = models.ObjIDMutable(ai.TenantAccountID)
		obj.UserID = models.ObjIDMutable(ai.UserID)
	} else if obj.AccountID == "" {
		e := h.eMissingMsg("accountId")
		return ops.NewAuditLogCreateDefault(int(e.Code)).WithPayload(e)
	}
	if obj.ParentNum != 0 {
		// copy ObjectID and ObjectType from the parent record, ignore passed values
		lParams := ops.AuditLogListParams{
			HTTPRequest: params.HTTPRequest,
			RecordNumGE: swag.Int32(obj.ParentNum),
			RecordNumLE: swag.Int32(obj.ParentNum),
		}
		resp := h.auditLogList(lParams)
		switch r := resp.(type) {
		case *ops.AuditLogListDefault:
			return ops.NewAuditLogCreateDefault(int(r.Payload.Code)).WithPayload(r.Payload)
		case *ops.AuditLogListOK:
			if len(r.Payload) != 1 {
				e := h.eMissingMsg("invalid parentNum")
				return ops.NewAuditLogCreateDefault(int(e.Code)).WithPayload(e)
			}
			obj.ObjectID = r.Payload[0].ObjectID
			obj.ObjectType = r.Payload[0].ObjectType
		}
	} else if obj.ObjectID == "" {
		e := h.eMissingMsg("either parentNum or objectId must be specified")
		return ops.NewAuditLogCreateDefault(int(e.Code)).WithPayload(e)
	} else {
		obj.ObjectType = strings.TrimSpace(strings.ToLower(obj.ObjectType))
		for _, action := range actionMap {
			if action.oType == obj.ObjectType {
				goto found
			}
		}
		e := h.eMissingMsg("invalid objectType")
		return ops.NewAuditLogCreateDefault(int(e.Code)).WithPayload(e)
	}
found:
	obj.Action = strings.TrimSpace(strings.ToLower(obj.Action))
	if obj.Action == "" {
		obj.Action = com.NoteAction
	}
	// TBD additional validations
	m := &LogRecord{
		Timestamp:       time.Now(),
		ParentNum:       obj.ParentNum,
		Classification:  obj.Classification,
		ObjectType:      obj.ObjectType,
		ObjectID:        idOrZeroUUID(string(obj.ObjectID)),
		ObjectName:      strings.TrimSpace(obj.Name),
		RefObjectID:     nilUUID,
		TenantAccountID: idOrZeroUUID(string(obj.TenantAccountID)),
		AccountID:       idOrZeroUUID(string(obj.AccountID)),
		AccountName:     obj.AccountName,
		UserID:          idOrZeroUUID(string(obj.UserID)),
		AuthIdentifier:  obj.AuthIdentifier,
		Action:          obj.Action,
		Error:           obj.Error,
		Message:         strings.TrimSpace(obj.Message),
	}
	var err error
	if obj.RecordNum, err = h.i.insert(ctx, m); err != nil {
		return ops.NewAuditLogCreateDefault(h.eCode(err)).WithPayload(h.eError(err))
	}
	obj.Timestamp = strfmt.DateTime(m.Timestamp)
	return ops.NewAuditLogCreateCreated().WithPayload(obj)
}

func (h *logHandler) auditLogList(params ops.AuditLogListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	var ai *auth.Info
	if cnv := ctx.Value(auth.InfoKey{}); cnv != nil {
		ai = cnv.(*auth.Info)
	} else {
		err := centrald.ErrorInternalError
		return ops.NewAuditLogListDefault(h.eCode(err)).WithPayload(h.eError(err))
	}
	if swag.BoolValue(params.Related) && swag.StringValue(params.ObjectID) == "" {
		e := h.eMissingMsg("objectId is required with related=true")
		return ops.NewAuditLogListDefault(int(e.Code)).WithPayload(e)
	}
	eitherAccount := false
	if !ai.Internal() {
		// restrict the filters to what is allowed based on the role of the caller
		systemAdminRole := ai.CapOK(centrald.ManageSpecialAccountsCap) == nil
		tenantAdminRole := ai.CapOK(centrald.ManageNormalAccountsCap) == nil
		accountAdminRole := ai.CapOK(centrald.AccountFetchAllRolesCap) == nil // might also be system or tenant admin
		if swag.StringValue(params.TenantAccountID) == "" {
			params.TenantAccountID = nil // normalize for later checks
			if tenantAdminRole {
				// tenant admin can see records for their own account and all subordinates
				if swag.StringValue(params.AccountID) == "" {
					params.AccountID = swag.String(ai.AccountID)
					params.TenantAccountID = swag.String(ai.AccountID)
					eitherAccount = true
				} else if *params.AccountID != ai.AccountID {
					// match records for a subordinate unless caller explicitly requested only the tenant admin account
					params.TenantAccountID = swag.String(ai.AccountID)
				}
			} else if systemAdminRole {
				// system admin can see records for system account and tenant admin accounts; both have nilUUID tenantAccountID
				params.TenantAccountID = swag.String(nilUUID)
			}
		} else if ai.TenantAccountID != *params.TenantAccountID && ai.CapOK(centrald.ManageNormalAccountsCap, models.ObjIDMutable(*params.TenantAccountID)) != nil {
			// caller is not a subordinate nor the tenant itself, skip query
			return ops.NewAuditLogListOK()
		}
		if swag.StringValue(params.AccountID) == "" {
			params.AccountID = nil // normalize for later checks
			// if caller is not a system or tenant admin, filter on their own account
			if !tenantAdminRole && !systemAdminRole && ai.AccountID != "" {
				params.AccountID = swag.String(ai.AccountID)
			}
		} else if ai.AccountID != *params.AccountID && !tenantAdminRole && !systemAdminRole {
			// different account specified and not a system or tenant admin: skip query
			return ops.NewAuditLogListOK()
		}
		if swag.StringValue(params.UserID) == "" {
			params.UserID = nil // normalize for later checks
			if !accountAdminRole {
				// query for all users is not allowed, only match records for the calling user
				params.UserID = swag.String(ai.UserID)
			}
		} else if *params.UserID != ai.UserID && !accountAdminRole {
			// not an admin and query is for some other user, skip
			return ops.NewAuditLogListOK()
		}
	}

	switch ver := h.c.TableVersion("AuditLog"); ver {
	case 1:
		// supported. TBD reorganize the code to have versioned list support
	default:
		err := h.c.Errorf("Unsupported AuditLog version %d", ver)
		h.c.Log.Error(err.M)
		return ops.NewAuditLogListDefault(h.eCode(err)).WithPayload(h.eError(err))
	}

	// The query construction depends on the value of params.Related.
	// When params.Related is false, all of the where conditions are combined in a single WHERE clause.
	// When params.Related is true, the conditions in where1 are used to select the initial set of records,
	// and the conditions in where2 are used to filter the final results.
	where1, where2, args, argIdx := []string{}, []string{}, []interface{}{}, 1

	// where1 clauses
	if swag.StringValue(params.ObjectID) != "" {
		where1 = append(where1, fmt.Sprintf("ObjectID = $%d", argIdx))
		args, argIdx = append(args, *params.ObjectID), argIdx+1
	}
	if eitherAccount {
		where1 = append(where1, fmt.Sprintf("(TenantAccountID = $%d OR AccountID = $%d)", argIdx, argIdx+1))
		args, argIdx = append(args, *params.TenantAccountID, *params.AccountID), argIdx+2
	} else {
		if params.TenantAccountID != nil {
			where1 = append(where1, fmt.Sprintf("TenantAccountID = $%d", argIdx))
			args, argIdx = append(args, *params.TenantAccountID), argIdx+1
		}
		if params.AccountID != nil {
			where1 = append(where1, fmt.Sprintf("AccountID = $%d", argIdx))
			args, argIdx = append(args, *params.AccountID), argIdx+1
		}
	}
	if params.UserID != nil {
		where1 = append(where1, fmt.Sprintf("UserID = $%d", argIdx))
		args, argIdx = append(args, *params.UserID), argIdx+1
	}

	// where2 clauses
	if params.RecordNumGE != nil {
		where2 = append(where2, fmt.Sprintf("RecordNum >= $%d", argIdx))
		args, argIdx = append(args, int(*params.RecordNumGE)), argIdx+1
	}
	if params.RecordNumLE != nil {
		where2 = append(where2, fmt.Sprintf("RecordNum <= $%d", argIdx))
		args, argIdx = append(args, int(*params.RecordNumLE)), argIdx+1
	}
	if params.TimeStampGE != nil {
		where2 = append(where2, fmt.Sprintf("TimeStamp >= $%d", argIdx))
		args, argIdx = append(args, (*params.TimeStampGE).String()), argIdx+1
	}
	if params.TimeStampLE != nil {
		where2 = append(where2, fmt.Sprintf("TimeStamp <= $%d", argIdx))
		args, argIdx = append(args, (*params.TimeStampLE).String()), argIdx+1
	}
	if swag.StringValue(params.Classification) != "" {
		where2 = append(where2, fmt.Sprintf("Classification = $%d", argIdx))
		args, argIdx = append(args, strings.ToLower(*params.Classification)), argIdx+1
	}
	if swag.StringValue(params.Name) != "" {
		where2 = append(where2, fmt.Sprintf("ObjectName = $%d", argIdx))
		args, argIdx = append(args, *params.Name), argIdx+1
	}
	if swag.StringValue(params.ObjectType) != "" {
		where2 = append(where2, fmt.Sprintf("ObjectType = $%d", argIdx))
		args, argIdx = append(args, strings.ToLower(*params.ObjectType)), argIdx+1
	}
	if len(params.Action) > 0 {
		actions := make([]string, 0, len(params.Action))
		for _, action := range params.Action {
			actions = append(actions, fmt.Sprintf("Action = $%d", argIdx))
			args, argIdx = append(args, strings.ToLower(action)), argIdx+1
		}
		where2 = append(where2, fmt.Sprintf("(%s)", strings.Join(actions, " OR ")))
	}
	if params.Error != nil {
		if *params.Error {
			where2 = append(where2, "Error")
		} else {
			where2 = append(where2, "NOT Error")
		}
	}

	q := "SELECT RecordNum, Timestamp, ParentNum, Classification, ObjectType, ObjectID, ObjectName, RefObjectID, TenantAccountID, AccountID, AccountName, UserID, AuthIdentifier, Action, Error, Message FROM "
	if swag.BoolValue(params.Related) {
		// where1 is guaranteed to at least contain the objectId condition
		q = "WITH RECURSIVE related AS ( SELECT * FROM AuditLog WHERE " + strings.Join(where1, " AND ") +
			" UNION SELECT a.* FROM AuditLog a INNER JOIN related r ON r.RefObjectID = a.ObjectID ) " + q + "related"
		if len(where2) > 0 {
			q += " WHERE " + strings.Join(where2, " AND ")
		}
	} else {
		q += "AuditLog"
		where := append(where1, where2...)
		if len(where) > 0 {
			q += " WHERE " + strings.Join(where, " AND ")
		}
	}
	q += " ORDER BY RecordNum ASC"
	if params.Count != nil {
		q += fmt.Sprintf(" LIMIT $%d", argIdx)
		args, argIdx = append(args, int(*params.Count)), argIdx+1
	}
	h.c.Log.Debugf("Query:%s Args:%v", q, args)
	h.c.mux.Lock()
	defer h.c.mux.Unlock()
	if h.c.db == nil {
		err := h.c.Errorf("database not connected")
		return ops.NewAuditLogListDefault(h.eCode(err)).WithPayload(h.eError(err))
	}
	rs, err := h.c.db.QueryContext(ctx, q, args...)
	if err != nil {
		h.c.Log.Errorf("AuditLog query error: %s", err.Error())
		e := h.c.Errorf("%s", err.Error())
		return ops.NewAuditLogListDefault(h.eCode(e)).WithPayload(h.eError(e))
	}
	defer rs.Close()
	list := []*models.AuditLogRecord{}
	for rs.Next() {
		rec := &models.AuditLogRecord{}
		var accountID, objectID, refObjectID, tenantAccountID, userID string // models.ObjID does not implement sql.Scanner, plus nilUUID needs to be handled
		if err = rs.Scan(&rec.RecordNum, &rec.Timestamp, &rec.ParentNum, &rec.Classification, &rec.ObjectType, &objectID, &rec.Name, &refObjectID, &tenantAccountID,
			&accountID, &rec.AccountName, &userID, &rec.AuthIdentifier, &rec.Action, &rec.Error, &rec.Message); err != nil {
			h.c.Log.Errorf("AuditLog scan error: %s", err.Error())
			e := h.c.Errorf("%s", err.Error())
			return ops.NewAuditLogListDefault(h.eCode(e)).WithPayload(h.eError(e))
		}
		if accountID == nilUUID {
			accountID = ""
		}
		if objectID == nilUUID {
			objectID = ""
		}
		if refObjectID == nilUUID {
			refObjectID = ""
		}
		if tenantAccountID == nilUUID {
			tenantAccountID = ""
		}
		if userID == nilUUID {
			userID = ""
		}
		rec.AccountID = models.ObjIDMutable(accountID)
		rec.ObjectID = models.ObjIDMutable(objectID)
		rec.RefObjectID = models.ObjIDMutable(refObjectID)
		rec.TenantAccountID = models.ObjIDMutable(tenantAccountID)
		rec.UserID = models.ObjIDMutable(userID)
		list = append(list, rec)
	}
	if err = rs.Err(); err != nil {
		h.c.Log.Errorf("AuditLog row error: %s", err.Error())
		e := h.c.Errorf("%s", err.Error())
		return ops.NewAuditLogListDefault(h.eCode(e)).WithPayload(h.eError(e))
	}
	return ops.NewAuditLogListOK().WithPayload(list)
}
