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
	"sync"
	"time"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// System object
// Note: models.System.Service is not persisted
type System struct {
	ObjMeta                  `bson:",inline"`
	Name                     string
	Description              string
	SnapshotCatalogPolicy    SnapshotCatalogPolicy
	SnapshotManagementPolicy SnapshotManagementPolicy
	VsrManagementPolicy      VsrManagementPolicy
	ClusterUsagePolicy       ClusterUsagePolicy
	UserPasswordPolicy       UserPasswordPolicy
	SystemTags               StringList
	ManagementHostCName      string
}

// System object defaults
const (
	SystemObjName              = "Nuvoloso"
	SystemObjDescription       = "The Nuvoloso System"
	SystemObjMinPasswordLength = int32(1)
	SystemObjSmpRDS            = int32(90 * 24 * time.Hour / time.Second)
	SystemObjVSRmpRDS          = int32(7 * 24 * time.Hour / time.Second)
)

// ToModel converts a datastore object to a model object
func (o *System) ToModel() *M.System {
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	// Service is not persisted, always return empty object
	// TBD persist messages across restart
	sc := NuvoService{}
	return &M.System{
		SystemAllOf0: M.SystemAllOf0{
			Meta:    (&o.ObjMeta).ToModel("System"),
			Service: sc.ToModel(),
		},
		SystemMutable: M.SystemMutable{
			Name:                     M.ObjName(o.Name),
			Description:              M.ObjDescription(o.Description),
			SnapshotCatalogPolicy:    o.SnapshotCatalogPolicy.ToModel(),
			SnapshotManagementPolicy: o.SnapshotManagementPolicy.ToModel(),
			VsrManagementPolicy:      o.VsrManagementPolicy.ToModel(),
			ClusterUsagePolicy:       o.ClusterUsagePolicy.ToModel(),
			UserPasswordPolicy:       o.UserPasswordPolicy.ToModel(),
			SystemTags:               *o.SystemTags.ToModel(),
			ManagementHostCName:      o.ManagementHostCName,
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *System) FromModel(mObj *M.System) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "System"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	o.SnapshotCatalogPolicy.FromModel(mObj.SnapshotCatalogPolicy)
	o.SnapshotManagementPolicy.FromModel(mObj.SnapshotManagementPolicy)
	o.VsrManagementPolicy.FromModel(mObj.VsrManagementPolicy)
	o.ClusterUsagePolicy.FromModel(mObj.ClusterUsagePolicy)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	userPasswordPolicy := mObj.UserPasswordPolicy
	if userPasswordPolicy == nil {
		userPasswordPolicy = &M.SystemMutableUserPasswordPolicy{}
	}
	o.UserPasswordPolicy.FromModel(userPasswordPolicy)
	o.SystemTags.FromModel(&mObj.SystemTags)
	o.ManagementHostCName = mObj.ManagementHostCName
}

// systemHandler is an ObjectDocumentHandler that implements the ds.SystemOps operations
type systemHandler struct {
	cName     string
	api       DBAPI
	crud      ObjectDocumentHandlerCRUD
	log       *logging.Logger
	mux       sync.Mutex
	systemID  string
	systemObj *System // cached copy
}

var _ = ds.SystemOps(&systemHandler{})
var _ = ObjectDocumentHandler(&systemHandler{})

var odhSystem = &systemHandler{
	cName: "system",
}

func init() {
	odhRegister(odhSystem.cName, odhSystem)
}

// objectDocumentHandler methods

func (op *systemHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *systemHandler) Initialize(ctx context.Context) error {
	if op.systemID != "" { // already fully initialized
		return nil
	}
	// ensure that the system object exists
	if err := op.crud.CreateIndexes(ctx, op); err != nil {
		return err
	}
	obj, err := op.intFetch(ctx)
	if err != nil {
		if err != ds.ErrorNotFound {
			return err
		}
		// create the System object
		obj = &M.System{
			SystemMutable: M.SystemMutable{
				Name:        SystemObjName,
				Description: SystemObjDescription,
				SnapshotCatalogPolicy: &M.SnapshotCatalogPolicy{
					Inherited: false, // empty but cannot be inherited
				},
				SnapshotManagementPolicy: &M.SnapshotManagementPolicy{
					RetentionDurationSeconds: swag.Int32(SystemObjSmpRDS),
				},
				VsrManagementPolicy: &M.VsrManagementPolicy{
					RetentionDurationSeconds: swag.Int32(SystemObjVSRmpRDS),
				},
				ClusterUsagePolicy: &M.ClusterUsagePolicy{
					AccountSecretScope:          common.AccountSecretScopeCluster,
					ConsistencyGroupName:        common.ConsistencyGroupNameDefault,
					VolumeDataRetentionOnDelete: common.VolumeDataRetentionOnDeleteRetain,
					Inherited:                   false,
				},
				UserPasswordPolicy: &M.SystemMutableUserPasswordPolicy{
					MinLength: SystemObjMinPasswordLength,
				},
			},
		}
		op.log.Infof("Creating system object")
		if obj, err = op.intCreate(ctx, obj); err != nil {
			return err
		}
	}
	op.systemID = string(obj.Meta.ID)
	return nil
}

func (op *systemHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *systemHandler) Ops() interface{} {
	return op
}

func (op *systemHandler) CName() string {
	return op.cName
}

func (op *systemHandler) Indexes() []mongo.IndexModel {
	return nil
}

func (op *systemHandler) NewObject() interface{} {
	return &System{}
}

// ds.SystemOps methods

func (op *systemHandler) intCreate(ctx context.Context, mObj *M.System) (*M.System, error) {
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &System{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	op.systemObj = dObj // cache it
	return dObj.ToModel(), nil
}

func (op *systemHandler) Fetch() (*M.System, error) {
	// op.systemObj is never reset to nil, so no need to hold the mux lock here
	if op.systemObj != nil {
		return op.systemObj.ToModel(), nil
	}
	// op.systemObj is cached by Initialize(). If it is nil Initialize must not have succeeded yet
	return nil, op.api.WrapError(fmt.Errorf("database not available"), false)
}

func (op *systemHandler) intFetch(ctx context.Context) (*M.System, error) {
	dObj := &System{}
	if err := op.crud.FindOne(ctx, op, bson.M{}, dObj); err != nil {
		return nil, err
	}
	op.systemObj = dObj // cache it
	return dObj.ToModel(), nil
}

func (op *systemHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.SystemMutable) (*M.System, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	return op.intUpdate(ctx, ua, param)
}

func (op *systemHandler) intUpdate(ctx context.Context, ua *ds.UpdateArgs, param *M.SystemMutable) (*M.System, error) {
	mObj := &M.System{SystemMutable: *param}
	// protect the default policies from deletion/invalid values
	if ua.IsModified("SnapshotCatalogPolicy") && (mObj.SnapshotCatalogPolicy == nil || mObj.SnapshotCatalogPolicy.Inherited == true) {
		return nil, &ds.Error{
			M: fmt.Sprintf("%s: snapshotCatalogPolicy", ds.ErrorInvalidData.M),
			C: ds.ErrorInvalidData.C,
		}
	}
	if ua.IsModified("SnapshotManagementPolicy") && (mObj.SnapshotManagementPolicy == nil || swag.Int32Value(mObj.SnapshotManagementPolicy.RetentionDurationSeconds) == 0) {
		return nil, &ds.Error{
			M: fmt.Sprintf("%s: snapshotManagementPolicy", ds.ErrorInvalidData.M),
			C: ds.ErrorInvalidData.C,
		}
	}
	if ua.IsModified("VsrManagementPolicy") && (mObj.VsrManagementPolicy == nil || swag.Int32Value(mObj.VsrManagementPolicy.RetentionDurationSeconds) == 0) {
		return nil, &ds.Error{
			M: fmt.Sprintf("%s: vsrManagementPolicy", ds.ErrorInvalidData.M),
			C: ds.ErrorInvalidData.C,
		}
	}
	if ua.IsModified("ClusterUsagePolicy") && (mObj.ClusterUsagePolicy == nil || mObj.ClusterUsagePolicy.Inherited == true) {
		return nil, &ds.Error{
			M: fmt.Sprintf("%s: clusterUsagePolicy", ds.ErrorInvalidData.M),
			C: ds.ErrorInvalidData.C,
		}
	}
	obj := &System{}
	obj.FromModel(mObj)
	ua.ID = op.systemID
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	op.mux.Lock()
	defer op.mux.Unlock()
	op.systemObj = obj
	return obj.ToModel(), nil
}
