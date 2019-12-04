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
	"context"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/go-openapi/swag"
)

func (c *csiComp) GetAccountFromSecret(ctx context.Context, args *csi.AccountFetchArgs) (*models.Account, error) {
	if err := args.Validate(ctx); err != nil {
		return nil, err
	}
	lParams := account.NewAccountListParams()
	lParams.CspDomainID = swag.String(args.CSPDomainID)
	lParams.ClusterID = swag.String(args.ClusterID)
	lParams.AccountSecret = swag.String(args.Secret)
	aList, err := c.app.OCrud.AccountList(ctx, lParams)
	if err != nil {
		return nil, err
	}
	if len(aList.Payload) != 1 {
		return nil, fmt.Errorf("account list failed")
	}
	return aList.Payload[0], nil
}
