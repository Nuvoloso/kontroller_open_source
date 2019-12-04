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


package mongods

import (
	"context"
	"fmt"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Account object
type Account struct {
	ObjMeta                  `bson:",inline"`
	Name                     string
	Description              string
	TenantAccountID          string
	Disabled                 bool
	AccountRoles             ObjIDList
	UserRoles                AuthRoleMap
	Secrets                  map[string]string
	ProtectionDomains        map[string]string
	SnapshotCatalogPolicy    SnapshotCatalogPolicy
	SnapshotManagementPolicy SnapshotManagementPolicy
	VsrManagementPolicy      VsrManagementPolicy
	Messages                 []TimestampedString
	Tags                     StringList
}

// ToModel converts a datastore object to a model object
func (o *Account) ToModel() *M.Account {
	if o.AccountRoles == nil {
		o.AccountRoles = make(ObjIDList, 0)
	}
	if o.UserRoles == nil {
		o.UserRoles = make(AuthRoleMap, 0)
	}
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	mObj := &M.Account{
		AccountAllOf0: M.AccountAllOf0{
			Meta:            (&o.ObjMeta).ToModel("Account"),
			AccountRoles:    *o.AccountRoles.ToModel(),
			TenantAccountID: M.ObjIDMutable(o.TenantAccountID),
		},
		AccountMutable: M.AccountMutable{
			Name:                     M.ObjName(o.Name),
			Description:              M.ObjDescription(o.Description),
			Tags:                     *o.Tags.ToModel(),
			Disabled:                 o.Disabled,
			UserRoles:                o.UserRoles.ToModel(),
			SnapshotCatalogPolicy:    o.SnapshotCatalogPolicy.ToModel(),
			SnapshotManagementPolicy: o.SnapshotManagementPolicy.ToModel(),
			VsrManagementPolicy:      o.VsrManagementPolicy.ToModel(),
		},
	}
	if o.Secrets != nil {
		mObj.Secrets = o.Secrets
	}
	mObj.Messages = make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		mObj.Messages[i] = msg.ToModel()
	}
	if len(o.ProtectionDomains) > 0 {
		mObj.ProtectionDomains = map[string]M.ObjIDMutable{}
		for k, v := range o.ProtectionDomains {
			mObj.ProtectionDomains[k] = M.ObjIDMutable(v)
		}
	}
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *Account) FromModel(mObj *M.Account) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Account"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	o.TenantAccountID = string(mObj.TenantAccountID)
	o.Disabled = mObj.Disabled
	if o.AccountRoles == nil {
		o.AccountRoles = make(ObjIDList, len(mObj.AccountRoles))
	}
	o.AccountRoles.FromModel(&mObj.AccountRoles)
	if o.UserRoles == nil {
		o.UserRoles = make(AuthRoleMap, len(mObj.UserRoles))
	}
	o.UserRoles.FromModel(mObj.UserRoles)
	o.Secrets = mObj.Secrets
	o.SnapshotCatalogPolicy.FromModel(mObj.SnapshotCatalogPolicy)
	o.SnapshotManagementPolicy.FromModel(mObj.SnapshotManagementPolicy)
	o.VsrManagementPolicy.FromModel(mObj.VsrManagementPolicy)
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, len(mObj.Messages))
	}
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	o.ProtectionDomains = nil
	if len(mObj.ProtectionDomains) > 0 {
		o.ProtectionDomains = make(map[string]string)
		for k, v := range mObj.ProtectionDomains {
			o.ProtectionDomains[k] = string(v)
		}
	}
}

// accountHandler is an ObjectDocumentHandler that implements the ds.AccountOps operations
type accountHandler struct {
	cName                string
	api                  DBAPI
	crud                 ObjectDocumentHandlerCRUD
	log                  *logging.Logger
	indexes              []mongo.IndexModel
	sysAccountID         string
	defTenantCreated     bool
	defSubAccountCreated bool
}

var _ = ds.AccountOps(&accountHandler{})
var _ = ObjectDocumentHandler(&accountHandler{})

var odhAccount = &accountHandler{
	cName: "account",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhAccount.cName, odhAccount)
}

// convertListParams is a helper to convert AccountListParams to bson.M
func (op *accountHandler) convertListParams(params account.AccountListParams) bson.M {
	qParams := bson.M{}
	orParams := newAndOrList()
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if params.TenantAccountID != nil && *params.TenantAccountID != "" {
		qParams[tenantAccountIDKey] = *params.TenantAccountID
	}
	if params.UserID != nil && *params.UserID != "" {
		qParams["userroles."+*params.UserID] = bson.M{"$exists": true}
	}
	if params.AccountSecret != nil && *params.AccountSecret != "" {
		qParams["secrets."+*params.AccountSecret] = bson.M{"$exists": true}
	} else { // if !AccountSecret
		if params.CspDomainID != nil && *params.CspDomainID != "" {
			orParams.append(
				bson.M{fmt.Sprintf("protectiondomains.%s", *params.CspDomainID): bson.M{"$exists": true}},
				bson.M{"snapshotcatalogpolicy.cspdomainid": *params.CspDomainID}, // relies on "" in inherited policies
			)
		}
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *accountHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *accountHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *accountHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	if err := op.api.MustBeInitializing(); err != nil {
		op.log.Error("Start:", err)
		return err
	}
	if op.sysAccountID == "" {
		// ensure that the system account exists and has the right system user role
		sysRoleID := string(odhRole.roles[ds.SystemAdminRole].Meta.ID)
		systemRole := M.AuthRole{RoleID: M.ObjIDMutable(sysRoleID)}
		dObj := &Account{}
		if err := op.crud.FindOne(ctx, op, bson.M{nameKey: ds.SystemAccount}, dObj); err != nil {
			if err != ds.ErrorNotFound {
				return err
			}
			// re/create the account
			mObj := &M.Account{
				AccountAllOf0: M.AccountAllOf0{
					AccountRoles: []M.ObjIDMutable{M.ObjIDMutable(sysRoleID)},
				},
				AccountMutable: M.AccountMutable{
					Name:        ds.SystemAccount,
					Description: ds.SystemAccountDescription,
					UserRoles: map[string]M.AuthRole{
						odhUser.adminUserID: systemRole,
					},
				},
			}
			op.log.Infof("Creating account '%s'", mObj.Name)
			o, err := op.intCreate(ctx, mObj)
			if err != nil {
				return err
			}
			op.sysAccountID = string(o.Meta.ID)
		} else {
			// now validate the account
			needUpdate := false
			if r, ok := dObj.UserRoles[odhUser.adminUserID]; !ok || r.Disabled || r.RoleID != sysRoleID {
				needUpdate = true
			}
			if needUpdate {
				dObj.UserRoles[odhUser.adminUserID] = AuthRole{RoleID: sysRoleID}
				ua := &ds.UpdateArgs{
					ID:      dObj.MetaObjID,
					Version: dObj.MetaVersion,
					Attributes: []ds.UpdateAttr{
						ds.UpdateAttr{
							Name: "UserRoles",
							Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
								ds.UpdateSet: ds.UpdateActionArgs{
									Fields: map[string]struct{}{
										odhUser.adminUserID: struct{}{},
									},
								},
							},
						},
					},
				}
				mObj := dObj.ToModel()
				op.log.Infof("Restoring system role in account '%s'", mObj.Name)
				if _, err := op.intUpdate(ctx, ua, &mObj.AccountMutable); err != nil {
					return err
				}
			}
			op.sysAccountID = dObj.MetaObjID
		}
	}
	if !op.defTenantCreated || !op.defSubAccountCreated {
		sysObj, _ := odhSystem.Fetch() // error impossible
		op.defTenantCreated = util.Contains(sysObj.SystemTags, common.SystemTagDefTenantCreated)
		op.defSubAccountCreated = util.Contains(sysObj.SystemTags, common.SystemTagDefSubAccountCreated)
		tenantAccountID := ""
		if !op.defTenantCreated {
			// Default tenant account is only created once. An admin can delete it and it will not be re-created.
			// If a failure occurs, the account object could exist without the systemObj.SystemTags being set.
			taRoleID := string(odhRole.roles[ds.TenantAdminRole].Meta.ID)
			taRole := M.AuthRole{RoleID: M.ObjIDMutable(taRoleID)}
			mObj := &M.Account{
				AccountAllOf0: M.AccountAllOf0{
					AccountRoles: []M.ObjIDMutable{M.ObjIDMutable(taRoleID)},
				},
				AccountMutable: M.AccountMutable{
					Name:        ds.DefTenantAccount,
					Description: ds.DefTenantDescription,
					UserRoles: map[string]M.AuthRole{
						odhUser.adminUserID: taRole,
					},
				},
			}
			op.log.Infof("Creating account '%s'", mObj.Name)
			tObj, err := op.intCreate(ctx, mObj)
			if err != nil && err != ds.ErrorExists {
				return err
			}
			if tObj != nil {
				tenantAccountID = string(tObj.Meta.ID)
			}
			sysObj.SystemTags = []string{common.SystemTagDefTenantCreated}
			ua := &ds.UpdateArgs{
				Attributes: []ds.UpdateAttr{
					{
						Name: "SystemTags",
						Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
							ds.UpdateAppend: ds.UpdateActionArgs{FromBody: true},
						},
					},
				},
			}
			if _, err := odhSystem.intUpdate(ctx, ua, &sysObj.SystemMutable); err != nil {
				return err
			}
			op.defTenantCreated = true
		}
		if tenantAccountID == "" && !op.defSubAccountCreated {
			// If we get here, the built-in tenant account was created previously, try to find it so we can create the built-in subordinate
			obj, err := op.intFetch(ctx, bson.M{"name": common.DefTenantAccount, "tenantaccountid": ""})
			if err == nil {
				tenantAccountID = string(obj.Meta.ID)
			} else if err != ds.ErrorNotFound {
				return err
			}
		}
		if !op.defSubAccountCreated {
			// The default subordinate account is only created once. An admin can delete it and it will not be re-created.
			// If a failure occurs, the account object could exist without the systemObj.SystemTags being set.
			// If the built-in tenant account object cannot be found, it must have already been deleted or modified, skip creating subordinate (but set tag).
			if tenantAccountID != "" {
				naRoleID := string(odhRole.roles[ds.AccountAdminRole].Meta.ID)
				nuRoleID := string(odhRole.roles[ds.AccountUserRole].Meta.ID)
				naRole := M.AuthRole{RoleID: M.ObjIDMutable(naRoleID)}
				mObj := &M.Account{
					AccountAllOf0: M.AccountAllOf0{
						AccountRoles:    []M.ObjIDMutable{M.ObjIDMutable(naRoleID), M.ObjIDMutable(nuRoleID)},
						TenantAccountID: M.ObjIDMutable(tenantAccountID),
					},
					AccountMutable: M.AccountMutable{
						Name:        ds.DefSubAccount,
						Description: ds.DefSubAccountDescription,
						UserRoles: map[string]M.AuthRole{
							odhUser.adminUserID: naRole,
						},
					},
				}
				op.log.Infof("Creating account '%s'", mObj.Name)
				if _, err := op.intCreate(ctx, mObj); err != nil && err != ds.ErrorExists {
					return err
				}
			} else {
				op.log.Infof("Skipped creating account '%s', built-in tenant was not found", ds.DefSubAccount)
			}
			sysObj.SystemTags = []string{common.SystemTagDefSubAccountCreated}
			ua := &ds.UpdateArgs{
				Attributes: []ds.UpdateAttr{
					{
						Name: "SystemTags",
						Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
							ds.UpdateAppend: ds.UpdateActionArgs{FromBody: true},
						},
					},
				},
			}
			if _, err := odhSystem.intUpdate(ctx, ua, &sysObj.SystemMutable); err != nil {
				return err
			}
			op.defSubAccountCreated = true
		}
	}
	return nil
}

func (op *accountHandler) Ops() interface{} {
	return op
}

func (op *accountHandler) CName() string {
	return op.cName
}

func (op *accountHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *accountHandler) NewObject() interface{} {
	return &Account{}
}

// ds.AccountOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *accountHandler) Count(ctx context.Context, params account.AccountListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *accountHandler) Create(ctx context.Context, mObj *M.Account) (*M.Account, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	return op.intCreate(ctx, mObj)
}

func (op *accountHandler) intCreate(ctx context.Context, mObj *M.Account) (*M.Account, error) {
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &Account{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	mObj = dObj.ToModel()
	return mObj, nil
}

func (op *accountHandler) Delete(ctx context.Context, mID string) error {
	if mID == op.sysAccountID {
		op.log.Warning("Attempt to delete the '%s' account", ds.SystemAccount)
		return ds.ErrorUnauthorizedOrForbidden
	}
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *accountHandler) Fetch(ctx context.Context, mID string) (*M.Account, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	return op.intFetch(ctx, bson.M{objKey: mID})
}

func (op *accountHandler) intFetch(ctx context.Context, filter bson.M) (*M.Account, error) {
	dObj := &Account{}
	if err := op.crud.FindOne(ctx, op, filter, dObj); err != nil {
		return nil, err
	}
	mObj := dObj.ToModel()
	return mObj, nil
}

func (op *accountHandler) List(ctx context.Context, params account.AccountListParams) ([]*M.Account, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.Account, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*Account) // or panic
		mObj := dObj.ToModel()
		list = append(list, mObj)
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *accountHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.AccountMutable) (*M.Account, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	return op.intUpdate(ctx, ua, param)
}

func (op *accountHandler) intUpdate(ctx context.Context, ua *ds.UpdateArgs, param *M.AccountMutable) (*M.Account, error) {
	mObj := &M.Account{AccountMutable: *param}
	obj := &Account{}
	obj.FromModel(mObj)
	if ua.ID == op.sysAccountID {
		if ua.IsModified("Name") {
			op.log.Warningf("Attempt to rename the %s account", ds.SystemAccount)
			return nil, ds.ErrorUnauthorizedOrForbidden
		} else if attr := ua.FindUpdateAttr("UserRoles"); attr != nil && attr.IsModified() {
			rl, present := param.UserRoles[odhUser.adminUserID]
			if attr.Actions[ds.UpdateRemove].FromBody {
				if present {
					op.log.Warningf("Attempt to remove the %s user in the %s account", ds.SystemUser, ds.SystemAccount)
					return nil, ds.ErrorUnauthorizedOrForbidden
				}
			} else if attr.Actions[ds.UpdateSet].FromBody && !present {
				op.log.Warningf("Attempt to remove the %s user in the %s account", ds.SystemUser, ds.SystemAccount)
				return nil, ds.ErrorUnauthorizedOrForbidden
			} else if present {
				action := attr.Actions[ds.UpdateAppend]
				if !action.FromBody {
					action = attr.Actions[ds.UpdateSet]
				}
				if _, exists := action.Fields[odhUser.adminUserID]; action.FromBody || exists {
					if rl.RoleID != M.ObjIDMutable(odhRole.roles[ds.SystemAdminRole].Meta.ID) || rl.Disabled {
						op.log.Warningf("Attempt to remove or disable the %s role from the %s user in the %s account", ds.SystemAdminRole, ds.SystemUser, ds.SystemAccount)
						return nil, ds.ErrorUnauthorizedOrForbidden
					}
				}
			}
		}
	}
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	mObj = obj.ToModel()
	return mObj, nil
}
