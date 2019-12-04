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


package crud

import (
	"context"
	"errors"
	"reflect"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
)

// See usage of VolumeSeriesUpdater or other updaters for the modify function description.
type modifyCallbackFn func(interface{}) (interface{}, error)
type objFetchFn func(context.Context, string) (interface{}, error)
type objUpdateFn func(context.Context, interface{}, *Updates) (interface{}, error)
type objMetaFn func(interface{}) *models.ObjMeta

type updaterArgs struct {
	typeName string
	modifyFn modifyCallbackFn
	fetchFn  objFetchFn
	updateFn objUpdateFn
	metaFn   objMetaFn
	oID      string
	items    *Updates
}

type objUpdater interface {
	objUpdate(ctx context.Context, oID string, items *Updates, ua *updaterArgs) (interface{}, error)
}

// objUpdater abstracts object updates with retry
func (c *Client) objUpdate(ctx context.Context, oID string, items *Updates, ua *updaterArgs) (interface{}, error) {
	var o, obj interface{}
	var err, lastErr error
	mustCallModifyFn := true
	o, err = ua.modifyFn(nil) // use initial cached object if available
	for i := 0; err == nil; i++ {
		if i != 0 {
			c.Log.Warningf("Retrying %s[%s] on update error: %s", ua.typeName, oID, lastErr.Error())
			o = nil // force a reload
		} else if o != nil && !reflect.ValueOf(o).IsNil() {
			mustCallModifyFn = false
		}
		if o == nil || reflect.ValueOf(o).IsNil() {
			obj, err = ua.fetchFn(ctx, oID)
			if err != nil {
				break
			}
			mustCallModifyFn = true
		}
		if mustCallModifyFn {
			o, err = ua.modifyFn(obj)
			if err != nil {
				break
			}
		}
		meta := ua.metaFn(o)
		if string(meta.ID) != oID {
			err = errors.New("invalid object")
			break
		}
		c.Log.Debugf("Updating %s[%s] %d:v%d %v", ua.typeName, oID, i, meta.Version, items)
		items.Version = int32(meta.Version)
		obj, err = ua.updateFn(ctx, o, items)
		if err == nil {
			return obj, nil
		}
		if err.Error() != com.ErrorIDVerNotFound {
			break
		}
		lastErr = err
		err = nil
	}
	return nil, err
}
