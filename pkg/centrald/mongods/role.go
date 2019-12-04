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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Role object
type Role struct {
	ObjMeta      `bson:",inline"`
	Name         string
	Capabilities CapabilityMap
}

// ToModel converts a datastore object to a model object
func (o *Role) ToModel() *M.Role {
	if o.Capabilities == nil {
		o.Capabilities = make(CapabilityMap, 0)
	}
	return &M.Role{
		RoleAllOf0: M.RoleAllOf0{Meta: (&o.ObjMeta).ToModel("Role")},
		RoleMutable: M.RoleMutable{
			Name:         M.ObjName(o.Name),
			Capabilities: o.Capabilities.ToModel(),
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *Role) FromModel(mObj *M.Role) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Role"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	if o.Capabilities == nil {
		o.Capabilities = make(CapabilityMap, len(mObj.Capabilities))
	}
	o.Capabilities.FromModel(mObj.Capabilities)
}

// roleHandler is an ObjectDocumentHandler that implements the ds.RoleOps operations
type roleHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
	roles   map[string]*M.Role
}

var _ = ds.RoleOps(&roleHandler{})
var _ = ObjectDocumentHandler(&roleHandler{})

var odhRole = &roleHandler{
	cName: "role",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
	roles: map[string]*M.Role{},
}

func init() {
	odhRegister(odhRole.cName, odhRole)
}

// objectDocumentHandler methods

func (op *roleHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *roleHandler) Initialize(ctx context.Context) error {
	if err := op.crud.CreateIndexes(ctx, op); err != nil {
		return err
	}
	if err := populateCollection(ctx, op.api, op); err != nil {
		return err
	}
	if _, ok := op.roles[ds.SystemAdminRole]; !ok {
		return fmt.Errorf("role %s does not exist", ds.SystemAdminRole)
	}
	if _, ok := op.roles[ds.TenantAdminRole]; !ok {
		return fmt.Errorf("role %s does not exist", ds.TenantAdminRole)
	}
	if _, ok := op.roles[ds.AccountAdminRole]; !ok {
		return fmt.Errorf("role %s does not exist", ds.AccountAdminRole)
	}
	if _, ok := op.roles[ds.AccountUserRole]; !ok {
		return fmt.Errorf("role %s does not exist", ds.AccountUserRole)
	}
	return nil
}

func (op *roleHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *roleHandler) Ops() interface{} {
	return op
}

func (op *roleHandler) CName() string {
	return op.cName
}

func (op *roleHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *roleHandler) NewObject() interface{} {
	return &Role{}
}

// Populator methods

func (op *roleHandler) Populate(ctx context.Context, fp string, buf []byte) error {
	mObj := &M.Role{}
	err := json.Unmarshal(buf, mObj)
	if err != nil {
		op.log.Errorf("Populate: json unmarshal failed for %s: %s", fp, err.Error())
		return err
	}
	if mObj.Name == "" {
		err = errors.New("non-empty name is required")
		op.log.Errorf("Populate: invalid role read from %s: %s", fp, err.Error())
		return err
	}
	dRole := &Role{}
	if err = op.crud.FindOne(ctx, op, bson.M{nameKey: string(mObj.Name)}, dRole); err != nil {
		if err != ds.ErrorNotFound {
			return err
		}
		mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
		dRole.FromModel(mObj)
		op.log.Infof("Creating built-in role '%s'", dRole.Name)
		err = op.crud.InsertOne(ctx, op, dRole)
	} else {
		caps := make(CapabilityMap, len(mObj.Capabilities))
		caps.FromModel(mObj.Capabilities)
		if !reflect.DeepEqual(caps, dRole.Capabilities) {
			dRole.Capabilities = caps
			ua := &ds.UpdateArgs{
				ID: dRole.MetaObjID,
				Attributes: []ds.UpdateAttr{
					{Name: "Capabilities", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
				},
			}
			op.log.Infof("Updating capabilities of built-in role '%s'", mObj.Name)
			err = op.crud.UpdateOne(ctx, op, dRole, ua)
		}
	}
	if err == nil {
		op.roles[dRole.Name] = dRole.ToModel()
	}
	return err
}

// ds.RoleOps methods

func (op *roleHandler) Fetch(mID string) (*M.Role, error) {
	// currently all roles are cached at startup
	for _, mObj := range op.roles {
		if mObj.Meta.ID == M.ObjID(mID) {
			return mObj, nil
		}
	}
	err := ds.ErrorNotFound
	op.log.Errorf("Role(%s): %s", mID, err)
	return nil, err
}

func (op *roleHandler) List(params role.RoleListParams) ([]*M.Role, error) {
	qParams := bson.M{}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	op.log.Info("qParams:", qParams)
	// currently all roles are cached at startup
	list := make([]*M.Role, 0, 4)
	if params.Name != nil && *params.Name != "" {
		mObj, exists := op.roles[*params.Name]
		if exists {
			list = append(list, mObj)
		}
	} else {
		for _, mObj := range op.roles {
			list = append(list, mObj)
		}
	}
	return list, nil
}
