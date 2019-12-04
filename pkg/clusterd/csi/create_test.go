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


package csi

import (
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fa "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCreateVolumeID(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	mockCtrl := gomock.NewController(t)
	appS := &fa.AppServant{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:            tl.Logger(),
			ServiceVersion: "0.6.2",
		},
		AppServant:    appS,
		ClusterClient: mockcluster.NewMockClient(mockCtrl),
	}
	c := &csiComp{}
	c.Init(app)

	// success
	fc := &fake.Client{}
	fc.RetVsNewIDVt = &models.ValueType{Kind: "STRING", Value: "someid"}
	fc.RetVsNewIDErr = nil
	c.app.OCrud = fc
	ret, err := c.CreateVolumeID(nil)
	assert.Nil(err)
	assert.Equal("someid", ret)

	// failure error
	fc = &fake.Client{}
	fc.RetVsNewIDVt = nil
	fc.RetVsNewIDErr = fmt.Errorf("bad error")
	c.app.OCrud = fc
	ret, err = c.CreateVolumeID(nil)
	assert.NotNil(err)
	assert.Empty(ret)
	assert.Regexp("unable to create Volume ID", err.Error())
}
