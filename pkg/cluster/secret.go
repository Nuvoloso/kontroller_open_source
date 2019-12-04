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


package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// Secrets is an interface for secrets in a Container Orchestrator
type Secrets interface {
	SecretFetch(ctx context.Context, sfa *SecretFetchArgs) (*SecretObj, error)
	SecretCreate(ctx context.Context, sca *SecretCreateArgs) (*SecretObj, error)
	SecretFormat(ctx context.Context, sca *SecretCreateArgs) (string, error)
	SecretFetchMV(ctx context.Context, sfa *SecretFetchArgs) (*SecretObjMV, error)
	SecretCreateMV(ctx context.Context, scaj *SecretCreateArgsMV) (*SecretObjMV, error)
	SecretFormatMV(ctx context.Context, scaj *SecretCreateArgsMV) (string, error)
}

// SecretCreateArgs contains the parameters required to create a new container orchestrator neutral secret
type SecretCreateArgs struct {
	Name      string
	Namespace string
	Data      string
}

// ValidateForFormat makes sure the input parameters of a secret are within bounds
func (sca *SecretCreateArgs) ValidateForFormat() error {
	if sca.Name == "" || sca.Data == "" {
		return fmt.Errorf("invalid values for Secret")
	}
	return nil
}

// SecretObj is a container orchestrator neutral structure to return secret information
type SecretObj struct {
	SecretCreateArgs
	Raw interface{} // CO specific
}

// SecretIntent describes the purpose of a multi-value secret
type SecretIntent int

// SecretIntent values
const (
	// A general key-value secret map
	SecretIntentGeneral SecretIntent = iota
	// Secret containing account identity only
	SecretIntentAccountIdentity
	// Secret containing account identity and volume customization data
	SecretIntentDynamicVolumeCustomization

	SecretIntentInvalid
)

// AccountVolumeData contains the data to identify the account and customize
// a dynamically provisioned volume. It is set in the SecretCreateArgsMV structure
// when the intent is SecretIntentAccountIdentity or SecretIntentDynamicVolumeCustomization.
type AccountVolumeData struct {
	AccountSecret               string   `json:"accountSecret"`
	ApplicationGroupName        string   `json:"applicationGroupName,omitempty"`
	ApplicationGroupDescription string   `json:"applicationGroupDescription,omitempty"`
	ApplicationGroupTags        []string `json:"applicationGroupTags,omitempty"`
	ConsistencyGroupName        string   `json:"consistencyGroupName,omitempty"`
	ConsistencyGroupDescription string   `json:"consistencyGroupDescription,omitempty"`
	ConsistencyGroupTags        []string `json:"consistencyGroupTags,omitempty"`
	VolumeTags                  []string `json:"volumeTags,omitempty"`
}

// SecretCreateArgsMV contains the parameters required to create a container orchestrator neutral multi-value secret
type SecretCreateArgsMV struct {
	Name              string
	Namespace         string
	Intent            SecretIntent
	Data              map[string]string // SecretIntentGeneral only
	CustomizationData AccountVolumeData // SecretIntentAccountIdentity, SecretIntentDynamicVolumeCustomization
}

// ValidateForFormat validates the SecretCreateArgsMV structure
func (sca *SecretCreateArgsMV) ValidateForFormat() error {
	inError := false
	if sca.Name == "" || sca.Intent < 0 || sca.Intent >= SecretIntentInvalid {
		inError = true
	} else if sca.Intent == SecretIntentDynamicVolumeCustomization || sca.Intent == SecretIntentAccountIdentity {
		customizedData := AccountVolumeData{AccountSecret: sca.CustomizationData.AccountSecret}
		if len(sca.CustomizationData.ApplicationGroupTags) == 0 {
			customizedData.ApplicationGroupTags = sca.CustomizationData.ApplicationGroupTags
		}
		if len(sca.CustomizationData.ConsistencyGroupTags) == 0 {
			customizedData.ConsistencyGroupTags = sca.CustomizationData.ConsistencyGroupTags
		}
		if len(sca.CustomizationData.VolumeTags) == 0 {
			customizedData.VolumeTags = sca.CustomizationData.VolumeTags
		}
		if sca.CustomizationData.AccountSecret == "" ||
			(sca.Intent == SecretIntentDynamicVolumeCustomization && reflect.DeepEqual(sca.CustomizationData, customizedData)) ||
			(sca.Intent == SecretIntentAccountIdentity && !reflect.DeepEqual(sca.CustomizationData, customizedData)) {
			inError = true
		}
	} else if len(sca.Data) == 0 {
		inError = true
	}
	if inError {
		return fmt.Errorf("invalid values for Secret")
	}
	return nil
}

// SecretObjMV returns multi-value secrets that were marshalled into SecretObj.Data
type SecretObjMV struct {
	SecretCreateArgsMV
	Raw interface{} // CO specific
}

// Unmarshal de-serializes the payload from a byte stream. Identity fields are ignored.
func (o *SecretObjMV) Unmarshal(data []byte) error {
	cd := &o.CustomizationData
	err := json.Unmarshal(data, cd)
	if err == nil && cd.AccountSecret != "" {
		minCd := &AccountVolumeData{AccountSecret: cd.AccountSecret}
		if !reflect.DeepEqual(cd, minCd) {
			o.Intent = SecretIntentDynamicVolumeCustomization
		} else {
			o.Intent = SecretIntentAccountIdentity
		}
	} else {
		if err = json.Unmarshal(data, &o.Data); err != nil {
			return err
		}
		o.Intent = SecretIntentGeneral
	}
	return nil
}

// Marshal serializes the payload. Identity fields are ignored.
func (o *SecretObjMV) Marshal() []byte {
	var secretBytes []byte
	if o.Intent == SecretIntentDynamicVolumeCustomization || o.Intent == SecretIntentAccountIdentity {
		secretBytes, _ = json.Marshal(o.CustomizationData)
	} else {
		secretBytes, _ = json.Marshal(o.Data)
	}
	return secretBytes
}

// SecretFetchArgs contains the parameters required to fetch a secret.
// Name - the name of the secret
// Namespace - the namespace containing the secret
type SecretFetchArgs struct {
	Name      string
	Namespace string
}
