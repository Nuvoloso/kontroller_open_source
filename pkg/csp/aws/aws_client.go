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


package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/awssdk"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
)

// Client implements the DomainClient for the AWS domain
type Client struct {
	csp           *CSP
	Timeout       time.Duration
	domID         models.ObjID
	attrs         map[string]models.ValueType
	credsProvider *CredsProvider
	creds         *credentials.Credentials
	client        awssdk.AWSClient
	session       *session.Session
	ec2           awssdk.EC2
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
		Timeout: time.Second * awsClientDefaultTimeoutSecs,
		domID:   id,
		attrs:   dObj.CspDomainAttributes,
		client:  awssdk.New(),
	}
	if err := c.clInit.clientInit(cl); err != nil {
		return nil, err
	}
	return cl, nil
}

// clientInit initializes an AWS client.
func (c *CSP) clientInit(cl *Client) error {
	if err := validateAttributes(cl.attrs, true); err != nil {
		return err
	}
	_, err := c.createSession(cl, true)
	return err
}

// CreateSession creates session
func (c *CSP) createSession(cl *Client, withRegion bool) (*session.Session, error) {
	cl.credsProvider = &CredsProvider{csp: c, cl: cl}
	cl.creds = credentials.NewCredentials(cl.credsProvider)
	cfg := aws.NewConfig().WithCredentials(cl.creds).WithLogLevel(aws.LogDebugWithHTTPBody).WithLogger(aws.LoggerFunc(c.dbg))
	if withRegion {
		reg, _ := cl.attrs[AttrRegion]
		cfg.Region = &reg.Value
	}
	session, err := cl.client.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("AWS session creation error: %w", err)
	}
	cl.session = session
	return session, nil
}

// validateAttributes validates attributes are present and of a proper kind
func validateAttributes(attrs map[string]models.ValueType, allAttrs bool) error {
	attrsOK := false
	if vt, ok := attrs[AttrAccessKeyID]; ok && vt.Kind == "STRING" {
		if vt, ok := attrs[AttrSecretAccessKey]; ok && vt.Kind == "SECRET" {
			if !allAttrs {
				attrsOK = true
			} else if vt, ok := attrs[AttrRegion]; ok && vt.Kind == "STRING" {
				if vt, ok := attrs[AttrAvailabilityZone]; ok && vt.Kind == "STRING" {
					attrsOK = true
				}
			}
		}
	}
	if !attrsOK {
		msg := "required domain attributes missing or invalid: need " + AttrAccessKeyID + "[S], " + AttrSecretAccessKey + "[E]"
		if allAttrs {
			msg += ", " + AttrRegion + "[S] and " + AttrAvailabilityZone + "[S]"
		}
		return fmt.Errorf(msg)
	}
	return nil
}

func validateIdentity(ctx context.Context, session *session.Session, client awssdk.AWSClient) error {
	svc := client.NewSTS(session)
	if _, err := svc.GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{}); err != nil {
		if aErr, ok := err.(awserr.RequestFailure); ok {
			switch aErr.Code() {
			case "InvalidClientTokenId":
				err = fmt.Errorf("incorrect %s", AttrAccessKeyID)
			case "SignatureDoesNotMatch":
				err = fmt.Errorf("incorrect %s", AttrSecretAccessKey)
			}
		}
		return fmt.Errorf("identity validation failed: %w", err)
	}
	return nil
}

type getNewClientFn func() awssdk.AWSClient

// getNewClientFn can be replaced during UT
var getNewClientHook getNewClientFn = getNewClient

func getNewClient() awssdk.AWSClient {
	return awssdk.New()
}

// ValidateCredential determines if the credentials are valid, returning nil on success
func (c *CSP) ValidateCredential(domType models.CspDomainTypeMutable, attrs map[string]models.ValueType) error {
	if string(domType) != CSPDomainType {
		return fmt.Errorf("invalid DomainType")
	}
	if err := validateAttributes(attrs, false); err != nil {
		return err
	}

	cl := &Client{
		attrs:  attrs,
		client: getNewClientHook(),
	}
	session, err := c.createSession(cl, false)
	if err != nil {
		return err
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*awsClientDefaultTimeoutSecs)
	defer cancelFn()

	err = validateIdentity(ctx, session, cl.client)
	return err
}

// ec2Client initializes the EC2 service client
func (cl *Client) ec2Client() awssdk.EC2 {
	cspMutex.Lock()
	defer cspMutex.Unlock()
	if cl.ec2 != nil {
		return cl.ec2
	}
	cl.ec2 = cl.client.NewEC2(cl.session)
	return cl.ec2
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
		secs = awsClientDefaultTimeoutSecs
	}
	cl.Timeout = time.Duration(secs) * time.Second
}

// Validate determines if the credentials, region and zone are valid, returning nil on success
func (cl *Client) Validate(ctx context.Context) error {
	reg, _ := cl.attrs[AttrRegion]
	zone, _ := cl.attrs[AttrAvailabilityZone]
	resolver := endpoints.DefaultResolver()
	partitions := resolver.(endpoints.EnumPartitions).Partitions()

	for _, p := range partitions {
		for id := range p.Regions() {
			if reg.Value == id {
				goto foundRegion
			}
		}
	}
	return fmt.Errorf("invalid %s [%s]", AttrRegion, reg.Value)
foundRegion:
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	if err := validateIdentity(ctx, cl.session, cl.client); err != nil {
		return err
	}
	ec2client := cl.ec2Client()
	zones, err := ec2client.DescribeAvailabilityZonesWithContext(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return fmt.Errorf("list %s for %s [%s] failed: %w", AttrAvailabilityZone, AttrRegion, reg.Value, err)
	}
	for _, z := range zones.AvailabilityZones {
		if aws.StringValue(z.ZoneName) == zone.Value {
			return nil
		}
	}
	return fmt.Errorf("invalid %s [%s] for %s [%s]", AttrAvailabilityZone, AttrRegion, zone.Value, reg.Value)
}

// CreateProtectionStore creates an AWS S3 bucket used for storing snapshots
func (cl *Client) CreateProtectionStore(ctx context.Context) (map[string]models.ValueType, error) {
	bucketName, _ := cl.attrs[AttrPStoreBucketName]
	reg, _ := cl.attrs[AttrRegion]
	var bucketConfig *s3.CreateBucketConfiguration
	if reg.Value != endpoints.UsEast1RegionID { // us-east-1 must not be specified as a location constraint; region is derived from the session only for us-east-1
		bucketConfig = &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(reg.Value),
		}
	}
	svc := cl.client.NewS3(cl.session)
	createBucketInput := &s3.CreateBucketInput{
		Bucket:                    aws.String(bucketName.Value),
		CreateBucketConfiguration: bucketConfig,
	}
	if _, err := svc.CreateBucketWithContext(ctx, createBucketInput); err != nil {
		if aErr, ok := err.(awserr.Error); ok {
			switch aErr.Code() {
			case s3.ErrCodeBucketAlreadyExists:
				err = fmt.Errorf("bucket %s already exists", bucketName)
			case s3.ErrCodeBucketAlreadyOwnedByYou:
				err = fmt.Errorf("bucket %s already owned by you", bucketName)
			}
		}
		return nil, fmt.Errorf("protection store creation failed: %w", err)
	}

	// restrict public access to objects in the bucket
	accessBlockInput := &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName.Value),
		PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}
	if _, err := svc.PutPublicAccessBlockWithContext(ctx, accessBlockInput); err != nil {
		return nil, fmt.Errorf("protection store put public access policy failed: %w", err)
	}
	return cl.attrs, nil
}

// CredsProvider provides access to the AWS API
type CredsProvider struct {
	csp *CSP
	cl  *Client
}

var _ = credentials.Provider(&CredsProvider{})

// Retrieve is required by the credentials.Provider interface
func (cp *CredsProvider) Retrieve() (credentials.Value, error) {
	cp.csp.dbg("Retrieve()")
	ak, _ := cp.cl.attrs[AttrAccessKeyID]
	sak, _ := cp.cl.attrs[AttrSecretAccessKey]
	return credentials.Value{
		AccessKeyID:     ak.Value,
		SecretAccessKey: sak.Value,
		ProviderName:    awsCredsProviderName,
	}, nil
}

// IsExpired is required by the credentials.Provider interface
func (cp *CredsProvider) IsExpired() bool {
	cp.csp.dbg("IsExpired()")
	return false
}
