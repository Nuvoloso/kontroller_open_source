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


package gc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/gcsdk"
	"google.golang.org/api/option"
)

// Client implements the DomainClient for the Google Cloud domain
type Client struct {
	csp            *CSP
	api            gcsdk.API
	Timeout        time.Duration
	domID          models.ObjID
	attrs          map[string]models.ValueType
	projectID      string
	computeService gcsdk.ComputeService
	vr             volumeRetriever
}

// Client returns a DomainClient
func (c *CSP) Client(dObj *models.CSPDomain) (csp.DomainClient, error) {
	if string(dObj.CspDomainType) != CSPDomainType {
		return nil, fmt.Errorf("invalid CSPDomain object")
	}
	id := models.ObjID("")
	if dObj.Meta != nil {
		id = dObj.Meta.ID
	}
	cl := &Client{
		csp:     c,
		api:     gcsdk.New(),
		Timeout: time.Second * clientDefaultTimeoutSecs,
		domID:   id,
		attrs:   dObj.CspDomainAttributes,
	}
	if err := c.clInit.clientInit(cl); err != nil {
		return nil, err
	}
	return cl, nil
}

// clientInit initializes an GC client.
func (c *CSP) clientInit(cl *Client) error {
	if err := validateAttributes(cl.attrs, true); err != nil {
		return err
	}
	var credValues map[string]string
	if err := json.Unmarshal([]byte(cl.attrs[AttrCred].Value), &credValues); err != nil {
		return fmt.Errorf("could not parse %s: %w", AttrCred, err)
	}
	cl.projectID = credValues[ProjectID]
	cl.vr = cl // self-reference
	return nil
}

// Type returns the CSPDomain type
func (cl *Client) Type() models.CspDomainTypeMutable {
	return cl.csp.Type()
}

// ID returns the domain object ID
func (cl *Client) ID() models.ObjID {
	return cl.domID
}

// SetTimeout sets the client timeout
func (cl *Client) SetTimeout(secs int) {
	if secs <= 0 {
		secs = clientDefaultTimeoutSecs
	}
	cl.Timeout = time.Duration(secs) * time.Second
}

// validateAttributes validates attributes are present and of a proper kind
func validateAttributes(attrs map[string]models.ValueType, allAttrs bool) error {
	// TBD: to finalize the attributes specifics
	attrsOK := false
	if vt, ok := attrs[AttrCred]; ok && vt.Kind == com.ValueTypeSecret {
		if !allAttrs {
			attrsOK = true
		} else if vt, ok := attrs[AttrZone]; ok && vt.Kind == com.ValueTypeString {
			attrsOK = true
		}
	}
	if !attrsOK {
		msg := "required domain attributes missing or invalid: need " + AttrCred + "[E]"
		if allAttrs {
			msg += ", " + AttrZone + "[S]"
		}
		return fmt.Errorf(msg)
	}
	return nil
}

func validateIdentity(ctx context.Context, cl *Client, attrs map[string]models.ValueType) error {
	_, err := cl.api.NewComputeService(ctx, option.WithCredentialsJSON([]byte(attrs[AttrCred].Value)))
	// TBD call some API that actually validates the provided credentials
	return err
}

// ValidateCredential determines if the credentials are valid, returning nil on success
func (c *CSP) ValidateCredential(domType models.CspDomainTypeMutable, attrs map[string]models.ValueType) error {
	if string(domType) != CSPDomainType {
		return fmt.Errorf("invalid DomainType")
	}
	if err := validateAttributes(attrs, false); err != nil {
		return err
	}
	var credValues map[string]string
	if err := json.Unmarshal([]byte(attrs[AttrCred].Value), &credValues); err != nil {
		return fmt.Errorf("could not parse %s: %w", AttrCred, err)
	}
	cl := &Client{
		csp:       c,
		api:       gcsdk.New(),
		attrs:     attrs,
		projectID: credValues[ProjectID],
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*clientDefaultTimeoutSecs)
	defer cancelFn()

	return validateIdentity(ctx, cl, attrs)
}

// Validate determines if the credentials, zone, etc. are valid, returning nil on success
func (cl *Client) Validate(ctx context.Context) error {
	zone, _ := cl.attrs[AttrZone]
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	service, err := cl.api.NewComputeService(ctx, option.WithCredentialsJSON([]byte(cl.attrs[AttrCred].Value)))
	if err != nil {
		return fmt.Errorf("failed to create GC compute service: %w", err)
	}
	if _, err = service.Zones().Get(cl.projectID, zone.Value).Context(ctx).Do(); err != nil {
		return err
	}
	cl.computeService = service
	return nil
}

// CreateProtectionStore creates Google Cloud bucket used for storing snapshots
func (cl *Client) CreateProtectionStore(ctx context.Context) (map[string]models.ValueType, error) {
	client, err := cl.api.NewStorageClient(ctx, option.WithCredentialsJSON([]byte(cl.attrs[AttrCred].Value)))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to GC storage client: %w", err)
	}
	bucketName := cl.attrs[AttrPStoreBucketName]
	bucket := client.Bucket(bucketName.Value)
	attrs := &storage.BucketAttrs{
		BucketPolicyOnly: storage.BucketPolicyOnly{Enabled: true}, // control access to all objects at the bucket level thru IAM
	}
	if err := bucket.Create(ctx, cl.projectID, attrs); err != nil {
		return nil, fmt.Errorf("protection store creation failed: %w", err)
	}
	return cl.attrs, nil
}
