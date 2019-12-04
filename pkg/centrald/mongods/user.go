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

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
	"golang.org/x/crypto/bcrypt"
)

// User object
type User struct {
	ObjMeta        `bson:",inline"`
	AuthIdentifier string
	Disabled       bool
	Password       string
	Profile        StringValueMap
}

// ToModel converts a datastore object to a model object
func (o *User) ToModel() *M.User {
	if o.Profile == nil {
		o.Profile = make(StringValueMap, 0)
	}
	return &M.User{
		UserAllOf0: M.UserAllOf0{Meta: (&o.ObjMeta).ToModel("User")},
		UserMutable: M.UserMutable{
			AuthIdentifier: o.AuthIdentifier,
			Disabled:       o.Disabled,
			// Password value is never exposed
			Profile: o.Profile.ToModel(),
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *User) FromModel(mObj *M.User) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "User"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.AuthIdentifier = mObj.AuthIdentifier
	o.Disabled = mObj.Disabled
	o.Password = mObj.Password
	if o.Profile == nil {
		o.Profile = make(StringValueMap, len(mObj.Profile))
	}
	o.Profile.FromModel(mObj.Profile)
}

// userHandler is an ObjectDocumentHandler that implements the ds.UserOps operations
type userHandler struct {
	cName       string
	api         DBAPI
	crud        ObjectDocumentHandlerCRUD
	log         *logging.Logger
	indexes     []mongo.IndexModel
	adminUserID string
}

var _ = ds.UserOps(&userHandler{})
var _ = ObjectDocumentHandler(&userHandler{})

var odhUser = &userHandler{
	cName: "user",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: authIdentifierKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhUser.cName, odhUser)
}

// objectDocumentHandler methods

func (op *userHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *userHandler) Initialize(ctx context.Context) error {
	if err := op.crud.CreateIndexes(ctx, op); err != nil {
		return err
	}
	return op.ensureBuiltInUsers(ctx)
}

type cryptPWFn func(password []byte, cost int) ([]byte, error)

var cryptPWHook cryptPWFn = bcrypt.GenerateFromPassword

func (op *userHandler) ensureBuiltInUsers(ctx context.Context) error {
	if op.adminUserID != "" {
		return nil
	}
	// ensure that the admin user exists and is not disabled
	dObj := &User{}
	if err := op.crud.FindOne(ctx, op, bson.M{authIdentifierKey: ds.SystemUser}, dObj); err != nil {
		if err != ds.ErrorNotFound {
			return err
		}
		// re/create the user
		pw, err := cryptPWHook([]byte(ds.SystemUser), 0)
		if err != nil {
			return err
		}
		mObj := &M.User{
			UserMutable: M.UserMutable{
				AuthIdentifier: ds.SystemUser,
				Password:       string(pw),
				Profile: map[string]M.ValueType{
					"userName": {Kind: "STRING", Value: ds.SystemUser},
				},
			},
		}
		op.log.Infof("Creating user '%s'", mObj.AuthIdentifier)
		o, err := op.intCreate(ctx, mObj)
		if err != nil {
			return err
		}
		op.adminUserID = string(o.Meta.ID)
		return nil
	}
	// now validate the user (if Cost of password cannot be determined, the hashed password is corrupt)
	if _, err := bcrypt.Cost([]byte(dObj.Password)); err != nil || dObj.Disabled {
		ua := &ds.UpdateArgs{
			ID:         dObj.MetaObjID,
			Version:    dObj.MetaVersion,
			Attributes: []ds.UpdateAttr{},
		}
		mObj := &M.UserMutable{Disabled: false}
		if dObj.Disabled {
			op.log.Infof("Re-enabling user '%s'", dObj.AuthIdentifier)
			ua.Attributes = append(ua.Attributes, ds.UpdateAttr{
				Name: "Disabled",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			})
		}
		if err != nil {
			pw, err := cryptPWHook([]byte(ds.SystemUser), 0)
			if err != nil {
				return err
			}
			mObj.Password = string(pw)
			op.log.Infof("Resetting password for user '%s'", dObj.AuthIdentifier)
			ua.Attributes = append(ua.Attributes, ds.UpdateAttr{
				Name: "Password",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			})
		}
		if _, err := op.intUpdate(ctx, ua, mObj); err != nil {
			return err
		}
	}
	op.adminUserID = dObj.MetaObjID
	return nil
}

func (op *userHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *userHandler) Ops() interface{} {
	return op
}

func (op *userHandler) CName() string {
	return op.cName
}

func (op *userHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *userHandler) NewObject() interface{} {
	return &User{}
}

// ds.UserOps methods

func (op *userHandler) Create(ctx context.Context, mObj *M.User) (*M.User, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	return op.intCreate(ctx, mObj)
}

func (op *userHandler) intCreate(ctx context.Context, mObj *M.User) (*M.User, error) {
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &User{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *userHandler) Delete(ctx context.Context, mID string) error {
	if mID == op.adminUserID {
		op.log.Warningf("Attempt to delete the '%s' user", ds.SystemUser)
		return ds.ErrorUnauthorizedOrForbidden
	}
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *userHandler) Fetch(ctx context.Context, mID string) (*M.User, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &User{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *userHandler) List(ctx context.Context, params user.UserListParams) ([]*M.User, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	qParams := bson.M{}
	if params.AuthIdentifier != nil && *params.AuthIdentifier != "" {
		qParams[authIdentifierKey] = *params.AuthIdentifier
	}
	if params.UserNamePattern != nil && *params.UserNamePattern != "" {
		qParams["profile.userName"] = primitive.Regex{Pattern: *params.UserNamePattern, Options: ""}
	}
	op.log.Info("qParams:", qParams)
	list := make([]*M.User, 0, 4)
	err := op.crud.FindAll(ctx, op, params, qParams, func(obj interface{}) {
		dObj := obj.(*User) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *userHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.UserMutable) (*M.User, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	return op.intUpdate(ctx, ua, param)
}

func (op *userHandler) intUpdate(ctx context.Context, ua *ds.UpdateArgs, param *M.UserMutable) (*M.User, error) {
	if ua.ID == op.adminUserID {
		if ua.IsModified("AuthIdentifier") && param.AuthIdentifier != ds.SystemUser {
			op.log.Warningf("Attempt to modify authIdentifier of %s user", ds.SystemUser)
			return nil, ds.ErrorUnauthorizedOrForbidden
		} else if ua.IsModified("Disabled") && param.Disabled {
			op.log.Warningf("Attempt to disable the %s user", ds.SystemUser)
			return nil, ds.ErrorUnauthorizedOrForbidden
		}
	}
	mObj := &M.User{UserMutable: *param}
	obj := &User{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
