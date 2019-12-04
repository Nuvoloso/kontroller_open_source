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


package azuresdk

import (
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
)

func TestAPI(t *testing.T) {
	assert := assert.New(t)

	subID := "subscriptionID"
	a := &fakeAuthorizer{}

	// Can only test factory methods

	api := New()
	assert.NotNil(api)
	sdk, ok := api.(*SDK)
	assert.True(ok)
	assert.NotNil(sdk)

	ccc := api.NewClientCredentialsConfig("clientID", "clientSecret", "tenantID")
	assert.NotNil(ccc)
	cccO, ok := ccc.(*ClientCredentialsConfig)
	assert.True(ok)
	assert.NotNil(cccO)
	assert.NotNil(cccO.CCC)

	assert.NotNil(ccc.Authorizer())

	spt, err := ccc.ServicePrincipalToken()
	assert.NoError(err)
	assert.NotNil(spt)
	sptO, ok := spt.(*ServicePrincipalToken)
	assert.True(ok)
	assert.NotNil(sptO)

	ac := api.NewAccountsClient(subID, a)
	assert.NotNil(ac)
	acO, ok := ac.(*AccountsClient)
	assert.True(ok)
	assert.NotNil(acO)
	assert.NotNil(acO.C)
	assert.Equal(a, acO.C.Authorizer)

	bc := api.NewBlobContainersClient(subID, a)
	assert.NotNil(bc)
	bcO, ok := bc.(*BlobContainersClient)
	assert.True(ok)
	assert.NotNil(bcO)
	assert.NotNil(bcO.C)
	assert.Equal(a, bcO.C.Authorizer)

	dc := api.NewDisksClient(subID, a)
	assert.NotNil(dc)
	dcO, ok := dc.(*DisksClient)
	assert.True(ok)
	assert.NotNil(dcO)
	assert.NotNil(dcO.C)
	assert.Equal(a, dcO.C.Authorizer)

	gc := api.NewGroupsClient(subID, a)
	assert.NotNil(gc)
	gcO, ok := gc.(*GroupsClient)
	assert.True(ok)
	assert.NotNil(gcO)
	assert.NotNil(gcO.C)
	assert.Equal(a, gcO.C.Authorizer)

	vc := api.NewVirtualMachinesClient(subID, a)
	assert.NotNil(vc)
	vcO, ok := vc.(*VirtualMachinesClient)
	assert.True(ok)
	assert.NotNil(vcO)
	assert.NotNil(vcO.C)
	assert.Equal(a, vcO.C.Authorizer)
}

type fakeAuthorizer struct{}

func (f *fakeAuthorizer) WithAuthorization() autorest.PrepareDecorator {
	return nil
}
