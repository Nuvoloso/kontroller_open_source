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


package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"golang.org/x/crypto/bcrypt"
)

// Register handlers
func (c *HandlerComp) userRegisterHandlers() {
	c.app.API.UserUserCreateHandler = ops.UserCreateHandlerFunc(c.userCreate)
	c.app.API.UserUserDeleteHandler = ops.UserDeleteHandlerFunc(c.userDelete)
	c.app.API.UserUserFetchHandler = ops.UserFetchHandlerFunc(c.userFetch)
	c.app.API.UserUserListHandler = ops.UserListHandlerFunc(c.userList)
	c.app.API.UserUserUpdateHandler = ops.UserUpdateHandlerFunc(c.userUpdate)
}

var nmUserMutable JSONToAttrNameMap

func (c *HandlerComp) userMutableNameMap() JSONToAttrNameMap {
	if nmUserMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmUserMutable == nil {
			nmUserMutable = c.makeJSONToAttrNameMap(models.UserMutable{})
		}
	}
	return nmUserMutable
}

func (c *HandlerComp) deriveAccountRoles(ctx context.Context, users []*models.User) error {
	lParams := account.AccountListParams{}
	if len(users) == 1 {
		lParams.UserID = swag.String(string(users[0].Meta.ID))
	}
	accounts, err := c.DS.OpsAccount().List(ctx, lParams)
	if err != nil {
		return err
	}
	// roles are currently fixed, no need to hold a read lock across these 2 List calls
	roles, err := c.DS.OpsRole().List(role.RoleListParams{})
	if err != nil {
		return err
	}
	for _, obj := range users {
		userID := string(obj.Meta.ID)
		obj.AccountRoles = make([]*models.UserAccountRole, 0, 1)
		if obj.Disabled {
			continue
		}
		for _, account := range accounts {
			if account.Disabled {
				continue
			}
			if userRole, ok := account.UserRoles[userID]; ok && !userRole.Disabled {
				roleID, roleName, tenantName := models.ObjID(userRole.RoleID), "", ""
				for _, role := range roles {
					if roleID == role.Meta.ID {
						roleName = string(role.Name)
						break
					}
				}
				if account.TenantAccountID != "" {
					for _, tenant := range accounts {
						if tenant.Meta.ID == models.ObjID(account.TenantAccountID) {
							tenantName = string(tenant.Name)
							break
						}
					}
				}
				userAccountRole := &models.UserAccountRole{
					AccountID:         account.Meta.ID,
					RoleID:            roleID,
					AccountName:       string(account.Name),
					TenantAccountName: tenantName,
					RoleName:          roleName,
				}
				obj.AccountRoles = append(obj.AccountRoles, userAccountRole)
			}
		}
	}
	return nil
}

// userFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) userFetchFilter(ai *auth.Info, obj *models.User) error {
	err := ai.CapOK(centrald.UserManagementCap)
	if err != nil {
		err = ai.UserOK(string(obj.Meta.ID))
	}
	return err
}

type cryptPWFn func(password []byte, cost int) ([]byte, error)

var cryptPWHook cryptPWFn = bcrypt.GenerateFromPassword

// userValidateCryptPassword checks that the given password satisfies the UserPasswordPolicy.
// Whitespace is trimmed from the password and on success, the password is replace with its encrypted form.
func (c *HandlerComp) userValidateCryptPassword(params *models.UserMutable, update bool) error {
	sysObj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return err
	}
	params.Password = strings.TrimSpace(params.Password)
	if len(params.Password) < int(sysObj.UserPasswordPolicy.MinLength) {
		errF := c.eMissingMsg
		if update {
			errF = c.eUpdateInvalidMsg
		}
		return errF("password must be at least %d characters long", sysObj.UserPasswordPolicy.MinLength)
	}
	pw, err := cryptPWHook([]byte(params.Password), 0)
	if err != nil {
		c.Log.Error("userValidateCryptPassword:", err)
		return &centrald.Error{M: fmt.Sprintf(centrald.ErrorInternalError.M+": %v", err), C: centrald.ErrorInternalError.C}
	}
	params.Password = string(pw)
	return nil
}

// Handlers

func (c *HandlerComp) userCreate(params ops.UserCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.AuthIdentifier == "" {
		err := c.eMissingMsg("authIdentifier")
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.userValidateCryptPassword(&params.Payload.UserMutable, false); err != nil {
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err := c.app.AuditLog.Ready(); err != nil {
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err := ai.CapOK(centrald.UserManagementCap); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.UserCreateAction, "", models.ObjName(params.Payload.AuthIdentifier), "", true, "Create unauthorized")
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsUser().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewUserCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Post(ctx, ai, centrald.UserCreateAction, obj.Meta.ID, models.ObjName(obj.AuthIdentifier), "", false, "Created")
	c.Log.Infof("User %s created [%s]", obj.AuthIdentifier, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewUserCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) userDelete(params ops.UserDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewUserDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *models.User
	if obj, err = c.DS.OpsUser().Fetch(ctx, params.ID); err == nil {
		if obj.AuthIdentifier == centrald.SystemUser {
			c.Log.Warning("Attempt to delete the '%s' user", centrald.SystemUser)
			err = centrald.ErrorUnauthorizedOrForbidden
		} else if err = c.app.AuditLog.Ready(); err == nil {
			if err = ai.CapOK(centrald.UserManagementCap); err == nil {
				c.Lock()
				defer c.Unlock()
				var n int
				if n, err = c.DS.OpsAccount().Count(ctx, account.AccountListParams{UserID: &params.ID}, 1); err == nil {
					if n != 0 {
						// TBD automatically remove user from all accounts
						err = &centrald.Error{M: "accounts are still associated with the user", C: centrald.ErrorExists.C}
					} else if err = c.DS.OpsUser().Delete(ctx, params.ID); err == nil {
						c.app.AuditLog.Post(ctx, ai, centrald.UserDeleteAction, obj.Meta.ID, models.ObjName(obj.AuthIdentifier), "", false, "Deleted")
						c.Log.Infof("User %s deleted [%s]", obj.AuthIdentifier, obj.Meta.ID)
						c.setDefaultObjectScope(params.HTTPRequest, obj)
						return ops.NewUserDeleteNoContent()
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.UserDeleteAction, obj.Meta.ID, models.ObjName(obj.AuthIdentifier), "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewUserDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) userFetch(params ops.UserFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewUserFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsUser().Fetch(ctx, params.ID)
	if err == nil {
		err = c.userFetchFilter(ai, obj)
	}
	if err == nil {
		err = c.deriveAccountRoles(ctx, []*models.User{obj})
	}
	if err != nil {
		return ops.NewUserFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewUserFetchOK().WithPayload(obj)
}

func (c *HandlerComp) userList(params ops.UserListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewUserListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	list, err := c.DS.OpsUser().List(ctx, params)
	if err == nil {
		ret := make([]*models.User, 0, len(list))
		for _, obj := range list {
			if c.userFetchFilter(ai, obj) == nil {
				ret = append(ret, obj)
			}
		}
		list = ret
	}
	if err == nil && len(list) > 0 {
		err = c.deriveAccountRoles(ctx, list)
	}
	if err != nil {
		return ops.NewUserListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewUserListOK().WithPayload(list)
}

func (c *HandlerComp) userUpdate(params ops.UserUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.userMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	authIdentifierChanged := false
	if ua.IsModified("AuthIdentifier") {
		if params.Payload.AuthIdentifier == "" {
			err := c.eUpdateInvalidMsg("non-empty authIdentifier is required")
			return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		authIdentifierChanged = true
	}
	c.RLock()
	defer c.RUnlock()
	obj, err := c.DS.OpsUser().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(obj.Meta.Version)
	} else if int32(obj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	pwChanged := ua.IsModified("Password")
	if pwChanged {
		if err = c.userValidateCryptPassword(params.Payload, true); err != nil {
			return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if err := c.app.AuditLog.Ready(); err != nil {
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// Note: middleware ensures we don't get here if the client user is disabled
	if obj.AuthIdentifier == centrald.SystemUser && pwChanged {
		err = ai.CapOK(centrald.SystemManagementCap)
	} else {
		err = ai.CapOK(centrald.UserManagementCap)
	}
	if err != nil {
		err = ai.UserOK(params.ID)
		if err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.UserUpdateAction, models.ObjID(params.ID), models.ObjName(obj.AuthIdentifier), "", true, "Update unauthorized")
			return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	obj, err = c.DS.OpsUser().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewUserUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	disabledChanged := ua.IsModified("Disabled")
	if authIdentifierChanged || disabledChanged {
		attrs := []string{}
		if authIdentifierChanged {
			attrs = append(attrs, "authIdentifier")
		}
		if disabledChanged {
			if params.Payload.Disabled {
				attrs = append(attrs, "disabled")
			} else {
				attrs = append(attrs, "enabled")
			}
		}
		if pwChanged {
			attrs = append(attrs, "password")
		}
		msg := fmt.Sprintf("Updated %s", strings.Join(attrs, ", "))
		c.app.AuditLog.Post(ctx, ai, centrald.UserUpdateAction, obj.Meta.ID, models.ObjName(obj.AuthIdentifier), "", false, msg)
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewUserUpdateOK().WithPayload(obj)
}
