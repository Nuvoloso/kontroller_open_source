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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	fa "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestGetAccountFromSecret(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	appS := &fa.AppServant{}
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		AppServant: appS,
	}
	c := &csiComp{}
	c.Init(app)

	// success
	acObj := &models.Account{}
	fc := &fake.Client{}
	fc.RetAccListObj = &account.AccountListOK{Payload: []*models.Account{acObj}}
	fc.RetAccListErr = nil
	c.app.OCrud = fc
	inArgs := &csi.AccountFetchArgs{
		Secret:      "test",
		CSPDomainID: "test",
		ClusterID:   "test",
	}
	ret, err := c.GetAccountFromSecret(nil, inArgs)
	assert.Nil(err)
	assert.NotNil(ret)
	assert.Equal(acObj, ret)

	// validate Error
	inArgs.Secret = ""
	ret, err = c.GetAccountFromSecret(nil, inArgs)
	assert.NotNil(err)
	assert.Nil(ret)
	inArgs.Secret = "test" // reset

	// list error
	fc.RetAccListErr = fmt.Errorf("list error")
	ret, err = c.GetAccountFromSecret(nil, inArgs)
	assert.NotNil(err)
	assert.Nil(ret)
	fc.RetAccListErr = nil // reset

	// multiple accounts in list response
	fc.RetAccListObj = &account.AccountListOK{Payload: []*models.Account{acObj, acObj}}
	ret, err = c.GetAccountFromSecret(nil, inArgs)
	assert.NotNil(err)
	assert.Nil(ret)

	// no accounts in list response
	fc.RetAccListObj = &account.AccountListOK{Payload: []*models.Account{}}
	ret, err = c.GetAccountFromSecret(nil, inArgs)
	assert.NotNil(err)
	assert.Nil(ret)
}
