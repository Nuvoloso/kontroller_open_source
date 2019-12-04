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


package mock

import (
	"sort"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/metrics"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/task"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

// AccountMatcher is a gomock.Matcher
type AccountMatcher struct {
	t                *testing.T
	CreateParam      *account.AccountCreateParams
	DeleteParam      *account.AccountDeleteParams
	FetchParam       *account.AccountFetchParams
	ListParam        *account.AccountListParams
	UpdateParam      *account.AccountUpdateParams
	SecretResetParam *account.AccountSecretResetParams
}

// NewAccountMatcher returns a new matcher
func NewAccountMatcher(t *testing.T, param interface{}) *AccountMatcher {
	m := &AccountMatcher{t: t}
	switch p := param.(type) {
	case *account.AccountCreateParams:
		m.CreateParam = p
	case *account.AccountDeleteParams:
		m.DeleteParam = p
	case *account.AccountFetchParams:
		m.FetchParam = p
	case *account.AccountListParams:
		m.ListParam = p
	case *account.AccountUpdateParams:
		m.UpdateParam = p
	case *account.AccountSecretResetParams:
		m.SecretResetParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *AccountMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *account.AccountCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *account.AccountDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *account.AccountFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *account.AccountListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *account.AccountUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	case *account.AccountSecretResetParams:
		// avoid matching the context
		o.SecretResetParam.Context = p.Context
		return assert.EqualValues(o.SecretResetParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *AccountMatcher) String() string {
	if o.CreateParam != nil {
		return "accountMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "accountMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "accountMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "accountMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "accountMatcher matches on update"
	} else if o.SecretResetParam != nil {
		return "accountMatcher matches on secretReset"
	}
	return "accountMatcher unknown context"
}

// ApplicationGroupMatcher is a gomock.Matcher
type ApplicationGroupMatcher struct {
	t           *testing.T
	CreateParam *application_group.ApplicationGroupCreateParams
	DeleteParam *application_group.ApplicationGroupDeleteParams
	FetchParam  *application_group.ApplicationGroupFetchParams
	ListParam   *application_group.ApplicationGroupListParams
	UpdateParam *application_group.ApplicationGroupUpdateParams
}

// NewApplicationGroupMatcher returns a new matcher
func NewApplicationGroupMatcher(t *testing.T, param interface{}) *ApplicationGroupMatcher {
	m := &ApplicationGroupMatcher{t: t}
	switch p := param.(type) {
	case *application_group.ApplicationGroupCreateParams:
		m.CreateParam = p
	case *application_group.ApplicationGroupDeleteParams:
		m.DeleteParam = p
	case *application_group.ApplicationGroupFetchParams:
		m.FetchParam = p
	case *application_group.ApplicationGroupListParams:
		m.ListParam = p
	case *application_group.ApplicationGroupUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *ApplicationGroupMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *application_group.ApplicationGroupCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *application_group.ApplicationGroupDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *application_group.ApplicationGroupFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *application_group.ApplicationGroupListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *application_group.ApplicationGroupUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *ApplicationGroupMatcher) String() string {
	if o.CreateParam != nil {
		return "applicationGroupMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "applicationGroupMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "applicationGroupMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "applicationGroupMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "applicationGroupMatcher matches on update"
	}
	return "applicationGroupMatcher unknown context"
}

// AuditLogMatcher is a gomock.Matcher
type AuditLogMatcher struct {
	t            *testing.T
	CreateParam  *audit_log.AuditLogCreateParams
	ListParam    *audit_log.AuditLogListParams
	DTimeStampGE time.Duration // List
}

// NewAuditLogMatcher returns a new matcher
func NewAuditLogMatcher(t *testing.T, param interface{}) *AuditLogMatcher {
	m := &AuditLogMatcher{t: t}
	switch p := param.(type) {
	case *audit_log.AuditLogCreateParams:
		m.CreateParam = p
	case *audit_log.AuditLogListParams:
		m.ListParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *AuditLogMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *audit_log.AuditLogCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *audit_log.AuditLogListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		var now = time.Now()
		var aTmGE time.Time
		if o.ListParam.TimeStampGE != nil {
			aTmGE = time.Time(*p.TimeStampGE)
			p.TimeStampGE = o.ListParam.TimeStampGE
		}
		return assert.EqualValues(o.ListParam, p) &&
			(o.ListParam.TimeStampGE == nil || assert.True(now.Add(-o.DTimeStampGE).After(aTmGE)))
	}
	return false
}

// String is from gomock.Matcher
func (o *AuditLogMatcher) String() string {
	if o.CreateParam != nil {
		return "auditLogMatcher matches on create"
	} else if o.ListParam != nil {
		return "auditLogMatcher matches on list"
	}
	return "auditLogMatcher unknown context"
}

// ClusterMatcher is a gomock.Matcher
type ClusterMatcher struct {
	t                  *testing.T
	AsfParam           *cluster.ClusterAccountSecretFetchParams
	CreateParam        *cluster.ClusterCreateParams
	FetchParam         *cluster.ClusterFetchParams
	ListParam          *cluster.ClusterListParams
	UpdateParam        *cluster.ClusterUpdateParams
	GetDeploymentParam *cluster.ClusterOrchestratorGetDeploymentParams
}

// NewClusterMatcher returns a new matcher
func NewClusterMatcher(t *testing.T, param interface{}) *ClusterMatcher {
	m := &ClusterMatcher{t: t}
	switch p := param.(type) {
	case *cluster.ClusterAccountSecretFetchParams:
		m.AsfParam = p
	case *cluster.ClusterCreateParams:
		m.CreateParam = p
	case *cluster.ClusterFetchParams:
		m.FetchParam = p
	case *cluster.ClusterListParams:
		m.ListParam = p
	case *cluster.ClusterUpdateParams:
		m.UpdateParam = p
	case *cluster.ClusterOrchestratorGetDeploymentParams:
		m.GetDeploymentParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *ClusterMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *cluster.ClusterAccountSecretFetchParams:
		// avoid matching the context
		o.AsfParam.Context = p.Context
		return assert.EqualValues(o.AsfParam, p)
	case *cluster.ClusterCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *cluster.ClusterFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *cluster.ClusterListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *cluster.ClusterUpdateParams:
		// // avoid matching the context
		o.UpdateParam.Context = p.Context
		if o.UpdateParam.Payload.Service != nil && p.Payload.Service != nil {
			o.UpdateParam.Payload.Service.HeartbeatTime = p.Payload.Service.HeartbeatTime
		}
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	case *cluster.ClusterOrchestratorGetDeploymentParams:
		// avoid matching the context
		o.GetDeploymentParam.Context = p.Context
		return assert.EqualValues(o.GetDeploymentParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *ClusterMatcher) String() string {
	if o.CreateParam != nil {
		return "clusterMatcher matches on create"
	} else if o.FetchParam != nil {
		return "clusterMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "clusterMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "clusterMatcher matches on update"
	} else if o.AsfParam != nil {
		return "clusterMatcher matches on accountSecretFetch"
	} else if o.GetDeploymentParam != nil {
		return "clusterMatcher matches on orchestratorGetDeployment"
	}
	return "clusterMatcher unknown context"
}

// CspCredentialMatcher is a gomock.Matcher
type CspCredentialMatcher struct {
	t           *testing.T
	CreateParam *csp_credential.CspCredentialCreateParams
	FetchParam  *csp_credential.CspCredentialFetchParams
	ListParam   *csp_credential.CspCredentialListParams
	UpdateParam *csp_credential.CspCredentialUpdateParams
}

// NewCspCredentialMatcher returns a new matcher
func NewCspCredentialMatcher(t *testing.T, param interface{}) *CspCredentialMatcher {
	m := &CspCredentialMatcher{t: t}
	switch p := param.(type) {
	case *csp_credential.CspCredentialCreateParams:
		m.CreateParam = p
	case *csp_credential.CspCredentialFetchParams:
		m.FetchParam = p
	case *csp_credential.CspCredentialListParams:
		m.ListParam = p
	case *csp_credential.CspCredentialUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *CspCredentialMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *csp_credential.CspCredentialCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *csp_credential.CspCredentialFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *csp_credential.CspCredentialListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *csp_credential.CspCredentialUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *CspCredentialMatcher) String() string {
	if o.CreateParam != nil {
		return "cspCredentialMatcher matches on create"
	} else if o.FetchParam != nil {
		return "cspCredentialMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "cspCredentialMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "cspCredentialMatcher matches on update"
	}
	return "cspCredentialMatcher unknown context"
}

// CspDomainMatcher is a gomock.Matcher
type CspDomainMatcher struct {
	t           *testing.T
	CreateParam *csp_domain.CspDomainCreateParams
	FetchParam  *csp_domain.CspDomainFetchParams
	ListParam   *csp_domain.CspDomainListParams
	UpdateParam *csp_domain.CspDomainUpdateParams
	SPCostParam *csp_domain.CspDomainServicePlanCostParams
}

// NewCspDomainMatcher returns a new matcher
func NewCspDomainMatcher(t *testing.T, param interface{}) *CspDomainMatcher {
	m := &CspDomainMatcher{t: t}
	switch p := param.(type) {
	case *csp_domain.CspDomainCreateParams:
		m.CreateParam = p
	case *csp_domain.CspDomainFetchParams:
		m.FetchParam = p
	case *csp_domain.CspDomainListParams:
		m.ListParam = p
	case *csp_domain.CspDomainUpdateParams:
		m.UpdateParam = p
	case *csp_domain.CspDomainServicePlanCostParams:
		m.SPCostParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *CspDomainMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *csp_domain.CspDomainCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *csp_domain.CspDomainFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *csp_domain.CspDomainListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *csp_domain.CspDomainServicePlanCostParams:
		// avoid matching the context
		o.SPCostParam.Context = p.Context
		return assert.EqualValues(o.SPCostParam, p)
	case *csp_domain.CspDomainUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *CspDomainMatcher) String() string {
	if o.CreateParam != nil {
		return "cspDomainMatcher matches on create"
	} else if o.FetchParam != nil {
		return "cspDomainMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "cspDomainMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "cspDomainMatcher matches on update"
	} else if o.SPCostParam != nil {
		return "cspDomainMatcher matches on service plan cost"
	}
	return "cspDomainMatcher unknown context"
}

// ConsistencyGroupMatcher is a gomock.Matcher
type ConsistencyGroupMatcher struct {
	t           *testing.T
	CreateParam *consistency_group.ConsistencyGroupCreateParams
	DeleteParam *consistency_group.ConsistencyGroupDeleteParams
	FetchParam  *consistency_group.ConsistencyGroupFetchParams
	ListParam   *consistency_group.ConsistencyGroupListParams
	UpdateParam *consistency_group.ConsistencyGroupUpdateParams
}

// NewConsistencyGroupMatcher returns a new matcher
func NewConsistencyGroupMatcher(t *testing.T, param interface{}) *ConsistencyGroupMatcher {
	m := &ConsistencyGroupMatcher{t: t}
	switch p := param.(type) {
	case *consistency_group.ConsistencyGroupCreateParams:
		m.CreateParam = p
	case *consistency_group.ConsistencyGroupDeleteParams:
		m.DeleteParam = p
	case *consistency_group.ConsistencyGroupFetchParams:
		m.FetchParam = p
	case *consistency_group.ConsistencyGroupListParams:
		m.ListParam = p
	case *consistency_group.ConsistencyGroupUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *ConsistencyGroupMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *consistency_group.ConsistencyGroupCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *consistency_group.ConsistencyGroupDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *consistency_group.ConsistencyGroupFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *consistency_group.ConsistencyGroupListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *consistency_group.ConsistencyGroupUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *ConsistencyGroupMatcher) String() string {
	if o.CreateParam != nil {
		return "consistencyGroupMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "consistencyGroupMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "consistencyGroupMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "consistencyGroupMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "consistencyGroupMatcher matches on update"
	}
	return "consistencyGroupMatcher unknown context"
}

// MetricMatcher is a gomock.Matcher
type MetricMatcher struct {
	t                    *testing.T
	StorageIOUpload      *metrics.StorageIOMetricUploadParams
	VolumeSeriesIOUpload *metrics.VolumeSeriesIOMetricUploadParams
}

// NewMetricMatcher returns a new matcher
func NewMetricMatcher(t *testing.T, param interface{}) *MetricMatcher {
	m := &MetricMatcher{t: t}
	switch p := param.(type) {
	case *metrics.StorageIOMetricUploadParams:
		m.StorageIOUpload = p
	case *metrics.VolumeSeriesIOMetricUploadParams:
		m.VolumeSeriesIOUpload = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *MetricMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *metrics.StorageIOMetricUploadParams:
		// avoid matching the context
		o.StorageIOUpload.Context = p.Context
		return assert.EqualValues(o.StorageIOUpload, p)
	case *metrics.VolumeSeriesIOMetricUploadParams:
		// avoid matching the context
		o.VolumeSeriesIOUpload.Context = p.Context
		return assert.EqualValues(o.VolumeSeriesIOUpload, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *MetricMatcher) String() string {
	if o.VolumeSeriesIOUpload != nil {
		return "VolumeSeriesIOUpload matches on create"
	} else if o.StorageIOUpload != nil {
		return "StorageIOUpload matches on create"
	}
	return "MetricMatcher unknown context"
}

// NodeMatcher is a gomock.Matcher
type NodeMatcher struct {
	t           *testing.T
	CreateParam *node.NodeCreateParams
	DeleteParam *node.NodeDeleteParams
	FetchParam  *node.NodeFetchParams
	ListParam   *node.NodeListParams
	UpdateParam *node.NodeUpdateParams
}

// NewNodeMatcher returns a new matcher
func NewNodeMatcher(t *testing.T, param interface{}) *NodeMatcher {
	m := &NodeMatcher{t: t}
	switch p := param.(type) {
	case *node.NodeCreateParams:
		m.CreateParam = p
	case *node.NodeDeleteParams:
		m.DeleteParam = p
	case *node.NodeFetchParams:
		m.FetchParam = p
	case *node.NodeListParams:
		m.ListParam = p
	case *node.NodeUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *NodeMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *node.NodeCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		if o.CreateParam.Payload.Service != nil && p.Payload.Service != nil {
			o.CreateParam.Payload.Service.HeartbeatTime = p.Payload.Service.HeartbeatTime
		}
		return assert.EqualValues(o.CreateParam, p)
	case *node.NodeDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *node.NodeFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *node.NodeListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *node.NodeUpdateParams:
		// avoid matching the context and the service state heartbeat time
		o.UpdateParam.Context = p.Context
		if o.UpdateParam.Payload.Service != nil && p.Payload.Service != nil {
			o.UpdateParam.Payload.Service.HeartbeatTime = p.Payload.Service.HeartbeatTime
		}
		return assert.EqualValues(o.UpdateParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *NodeMatcher) String() string {
	if o.CreateParam != nil {
		return "nodeMatcher matches on create"
	} else if o.FetchParam != nil {
		return "nodeMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "nodeMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "nodeMatcher matches on update"
	}
	return "nodeMatcher unknown context"
}

// PoolMatcher is a gomock.Matcher
type PoolMatcher struct {
	t           *testing.T
	CreateParam *pool.PoolCreateParams
	FetchParam  *pool.PoolFetchParams
	ListParam   *pool.PoolListParams
	DeleteParam *pool.PoolDeleteParams
	UpdateParam *pool.PoolUpdateParams
}

// NewPoolMatcher returns a new matcher
func NewPoolMatcher(t *testing.T, param interface{}) *PoolMatcher {
	m := &PoolMatcher{t: t}
	switch p := param.(type) {
	case *pool.PoolCreateParams:
		m.CreateParam = p
	case *pool.PoolFetchParams:
		m.FetchParam = p
	case *pool.PoolListParams:
		m.ListParam = p
	case *pool.PoolDeleteParams:
		m.DeleteParam = p
	case *pool.PoolUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *PoolMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *pool.PoolCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *pool.PoolFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *pool.PoolListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *pool.PoolDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *pool.PoolUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *PoolMatcher) String() string {
	if o.CreateParam != nil {
		return "pool matches on create"
	} else if o.FetchParam != nil {
		return "pool matches on fetch"
	} else if o.ListParam != nil {
		return "pool matches on list"
	} else if o.UpdateParam != nil {
		return "pool matches on update"
	}
	return "pool unknown context"
}

// ProtectionDomainMatcher is a gomock.Matcher
type ProtectionDomainMatcher struct {
	t           *testing.T
	CreateParam *protection_domain.ProtectionDomainCreateParams
	FetchParam  *protection_domain.ProtectionDomainFetchParams
	ListParam   *protection_domain.ProtectionDomainListParams
	DeleteParam *protection_domain.ProtectionDomainDeleteParams
	UpdateParam *protection_domain.ProtectionDomainUpdateParams
}

// NewProtectionDomainMatcher returns a new matcher
func NewProtectionDomainMatcher(t *testing.T, param interface{}) *ProtectionDomainMatcher {
	m := &ProtectionDomainMatcher{t: t}
	switch p := param.(type) {
	case *protection_domain.ProtectionDomainCreateParams:
		m.CreateParam = p
	case *protection_domain.ProtectionDomainFetchParams:
		m.FetchParam = p
	case *protection_domain.ProtectionDomainListParams:
		m.ListParam = p
	case *protection_domain.ProtectionDomainDeleteParams:
		m.DeleteParam = p
	case *protection_domain.ProtectionDomainUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *ProtectionDomainMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *protection_domain.ProtectionDomainCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *protection_domain.ProtectionDomainFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *protection_domain.ProtectionDomainListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *protection_domain.ProtectionDomainDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *protection_domain.ProtectionDomainUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *ProtectionDomainMatcher) String() string {
	if o.CreateParam != nil {
		return "ProtectionDomain matches on create"
	} else if o.FetchParam != nil {
		return "ProtectionDomain matches on fetch"
	} else if o.ListParam != nil {
		return "ProtectionDomain matches on list"
	} else if o.UpdateParam != nil {
		return "ProtectionDomain matches on update"
	}
	return "ProtectionDomain unknown context"
}

// RoleMatcher is a gomock.Matcher
type RoleMatcher struct {
	t         *testing.T
	ListParam *role.RoleListParams
}

// NewRoleMatcher returns a new matcher
func NewRoleMatcher(t *testing.T, param interface{}) *RoleMatcher {
	m := &RoleMatcher{t: t}
	switch p := param.(type) {
	case *role.RoleListParams:
		m.ListParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *RoleMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *role.RoleListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *RoleMatcher) String() string {
	if o.ListParam != nil {
		return "roleMatcher matches on list"
	}
	return "roleMatcher unknown context"
}

// ServicePlanAllocationMatcher is a gomock.Matcher
type ServicePlanAllocationMatcher struct {
	t              *testing.T
	CreateParam    *service_plan_allocation.ServicePlanAllocationCreateParams
	DeleteParam    *service_plan_allocation.ServicePlanAllocationDeleteParams
	FetchParam     *service_plan_allocation.ServicePlanAllocationFetchParams
	ListParam      *service_plan_allocation.ServicePlanAllocationListParams
	UpdateParam    *service_plan_allocation.ServicePlanAllocationUpdateParams
	CustomizeParam *service_plan_allocation.ServicePlanAllocationCustomizeProvisioningParams
}

// NewServicePlanAllocationMatcher returns a new matcher
func NewServicePlanAllocationMatcher(t *testing.T, param interface{}) *ServicePlanAllocationMatcher {
	m := &ServicePlanAllocationMatcher{t: t}
	switch p := param.(type) {
	case *service_plan_allocation.ServicePlanAllocationCreateParams:
		m.CreateParam = p
	case *service_plan_allocation.ServicePlanAllocationDeleteParams:
		m.DeleteParam = p
	case *service_plan_allocation.ServicePlanAllocationFetchParams:
		m.FetchParam = p
	case *service_plan_allocation.ServicePlanAllocationListParams:
		m.ListParam = p
	case *service_plan_allocation.ServicePlanAllocationUpdateParams:
		m.UpdateParam = p
	case *service_plan_allocation.ServicePlanAllocationCustomizeProvisioningParams:
		m.CustomizeParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *ServicePlanAllocationMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *service_plan_allocation.ServicePlanAllocationCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *service_plan_allocation.ServicePlanAllocationDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *service_plan_allocation.ServicePlanAllocationFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *service_plan_allocation.ServicePlanAllocationListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *service_plan_allocation.ServicePlanAllocationCustomizeProvisioningParams:
		// avoid matching the context
		o.CustomizeParam.Context = p.Context
		return assert.EqualValues(o.CustomizeParam, p)
	case *service_plan_allocation.ServicePlanAllocationUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(swag.Int32Value(o.UpdateParam.Version), swag.Int32Value(p.Version)) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *ServicePlanAllocationMatcher) String() string {
	if o.CreateParam != nil {
		return "ServicePlanAllocationMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "ServicePlanAllocationMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "ServicePlanAllocationMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "ServicePlanAllocationMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "ServicePlanAllocationMatcher matches on update"
	} else if o.CustomizeParam != nil {
		return "ServicePlanAllocationMatcher matches on customizeProvisioning"
	}
	return "ServicePlanAllocationMatcher unknown context"
}

// ServicePlanMatcher is a gomock.Matcher
type ServicePlanMatcher struct {
	t           *testing.T
	FetchParam  *service_plan.ServicePlanFetchParams
	ListParam   *service_plan.ServicePlanListParams
	UpdateParam *service_plan.ServicePlanUpdateParams
	// TBD: Clone, Publish, Retire
}

// NewServicePlanMatcher returns a new matcher
func NewServicePlanMatcher(t *testing.T, param interface{}) *ServicePlanMatcher {
	m := &ServicePlanMatcher{t: t}
	switch p := param.(type) {
	case *service_plan.ServicePlanFetchParams:
		m.FetchParam = p
	case *service_plan.ServicePlanListParams:
		m.ListParam = p
	case *service_plan.ServicePlanUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *ServicePlanMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *service_plan.ServicePlanFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *service_plan.ServicePlanListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *service_plan.ServicePlanUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		return assert.EqualValues(o.UpdateParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *ServicePlanMatcher) String() string {
	if o.FetchParam != nil {
		return "servicePlanMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "servicePlanMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "servicePlanMatcher matches on update"
	}
	return "servicePlanMatcher unknown context"
}

// SnapshotMatcher is a gomock.Matcher
type SnapshotMatcher struct {
	t                  *testing.T
	CreateParam        *snapshot.SnapshotCreateParams
	DeleteParam        *snapshot.SnapshotDeleteParams
	FetchParam         *snapshot.SnapshotFetchParams
	ListParam          *snapshot.SnapshotListParams
	UpdateParam        *snapshot.SnapshotUpdateParams
	DSnapTimeGE        time.Duration // list
	DSnapTimeLE        time.Duration // list
	DDeleteAfterTimeLE time.Duration // list
}

// NewSnapshotMatcher returns a new matcher
func NewSnapshotMatcher(t *testing.T, param interface{}) *SnapshotMatcher {
	m := &SnapshotMatcher{t: t}
	switch p := param.(type) {
	case *snapshot.SnapshotCreateParams:
		m.CreateParam = p
	case *snapshot.SnapshotDeleteParams:
		m.DeleteParam = p
	case *snapshot.SnapshotFetchParams:
		m.FetchParam = p
	case *snapshot.SnapshotListParams:
		m.ListParam = p
	case *snapshot.SnapshotUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *SnapshotMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *snapshot.SnapshotCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *snapshot.SnapshotDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *snapshot.SnapshotFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *snapshot.SnapshotListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context

		var now = time.Now()
		var tmGE, tmLE, datLE time.Time
		if o.ListParam.SnapTimeGE != nil {
			tmGE = time.Time(*p.SnapTimeGE)
			p.SnapTimeGE = o.ListParam.SnapTimeGE
		}
		if o.ListParam.SnapTimeLE != nil {
			tmLE = time.Time(*p.SnapTimeLE)
			p.SnapTimeLE = o.ListParam.SnapTimeLE
		}
		if o.ListParam.DeleteAfterTimeLE != nil {
			datLE = time.Time(*p.DeleteAfterTimeLE)
			p.DeleteAfterTimeLE = o.ListParam.DeleteAfterTimeLE
		}
		datMatch := o.DDeleteAfterTimeLE == 0 || (assert.True(now.Add(o.DDeleteAfterTimeLE).After(datLE)))
		return assert.EqualValues(o.ListParam, p) &&
			(o.ListParam.SnapTimeGE == nil || assert.True(now.Add(-o.DSnapTimeGE).After(tmGE))) &&
			(o.ListParam.SnapTimeLE == nil || assert.True(now.Add(-o.DSnapTimeLE).After(tmLE))) && datMatch
	case *snapshot.SnapshotUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *SnapshotMatcher) String() string {
	if o.CreateParam != nil {
		return "snapshotMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "snapshotMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "snapshotMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "snapshotMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "snapshotMatcher matches on update"
	}
	return "snapshotMatcher unknown context"
}

// StorageMatcher is a gomock.Matcher
type StorageMatcher struct {
	t           *testing.T
	CreateParam *storage.StorageCreateParams
	DeleteParam *storage.StorageDeleteParams
	FetchParam  *storage.StorageFetchParams
	ListParam   *storage.StorageListParams
	UpdateParam *storage.StorageUpdateParams
}

// NewStorageMatcher returns a new matcher
func NewStorageMatcher(t *testing.T, param interface{}) *StorageMatcher {
	m := &StorageMatcher{t: t}
	switch p := param.(type) {
	case *storage.StorageCreateParams:
		m.CreateParam = p
	case *storage.StorageDeleteParams:
		m.DeleteParam = p
	case *storage.StorageFetchParams:
		m.FetchParam = p
	case *storage.StorageListParams:
		m.ListParam = p
	case *storage.StorageUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *StorageMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *storage.StorageCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *storage.StorageDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *storage.StorageFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *storage.StorageListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *storage.StorageUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *StorageMatcher) String() string {
	if o.CreateParam != nil {
		return "storageMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "storageMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "storageMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "storageMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "storageMatcher matches on update"
	}
	return "storageMatcher unknown context"
}

// StorageFormulaMatcher is a gomock.Matcher
type StorageFormulaMatcher struct {
	t         *testing.T
	ListParam *storage_formula.StorageFormulaListParams
}

// NewStorageFormulaMatcher returns a new matcher
func NewStorageFormulaMatcher(t *testing.T, param interface{}) *StorageFormulaMatcher {
	m := &StorageFormulaMatcher{t: t}
	switch p := param.(type) {
	case *storage_formula.StorageFormulaListParams:
		m.ListParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *StorageFormulaMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *storage_formula.StorageFormulaListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *StorageFormulaMatcher) String() string {
	if o.ListParam != nil {
		return "storageFormulaMatcher matches on list"
	}
	return "storageFormulaMatcher unknown context"
}

// StorageRequestMatcher is a gomock.Matcher
type StorageRequestMatcher struct {
	t                       *testing.T
	CreateParam             *storage_request.StorageRequestCreateParams
	DeleteParam             *storage_request.StorageRequestDeleteParams
	FetchParam              *storage_request.StorageRequestFetchParams
	ListParam               *storage_request.StorageRequestListParams
	UpdateParam             *storage_request.StorageRequestUpdateParams
	D                       time.Duration // Create CompleteByTime
	DActiveOrTimeModifiedGE time.Duration // List
	ZeroMsgTime             bool
}

// NewStorageRequestMatcher returns a new matcher
func NewStorageRequestMatcher(t *testing.T, param interface{}) *StorageRequestMatcher {
	m := &StorageRequestMatcher{t: t}
	switch p := param.(type) {
	case *storage_request.StorageRequestCreateParams:
		m.CreateParam = p
	case *storage_request.StorageRequestDeleteParams:
		m.DeleteParam = p
	case *storage_request.StorageRequestFetchParams:
		m.FetchParam = p
	case *storage_request.StorageRequestListParams:
		m.ListParam = p
	case *storage_request.StorageRequestUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *StorageRequestMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *storage_request.StorageRequestCreateParams:
		// avoid matching the context and time
		o.CreateParam.Context = p.Context
		cb := time.Time(p.Payload.CompleteByTime)
		p.Payload.CompleteByTime = o.CreateParam.Payload.CompleteByTime
		return assert.EqualValues(o.CreateParam, p) &&
			assert.True(time.Now().Add(o.D).After(cb))
	case *storage_request.StorageRequestDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *storage_request.StorageRequestFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *storage_request.StorageRequestListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		var now = time.Now()
		var aTmGE time.Time
		if o.ListParam.ActiveOrTimeModifiedGE != nil {
			aTmGE = time.Time(*p.ActiveOrTimeModifiedGE)
			p.ActiveOrTimeModifiedGE = o.ListParam.ActiveOrTimeModifiedGE
		}
		return assert.EqualValues(o.ListParam, p) &&
			(o.ListParam.ActiveOrTimeModifiedGE == nil || assert.True(now.Add(-o.DActiveOrTimeModifiedGE).After(aTmGE)))
	case *storage_request.StorageRequestUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		if o.ZeroMsgTime {
			mt := strfmt.DateTime(time.Now())
			for i := range p.Payload.RequestMessages {
				p.Payload.RequestMessages[i].Time = mt
			}
			for i := range o.UpdateParam.Payload.RequestMessages {
				o.UpdateParam.Payload.RequestMessages[i].Time = mt
			}
		}
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *StorageRequestMatcher) String() string {
	if o.CreateParam != nil {
		return "storageRequestMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "storageRequestMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "storageRequestMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "storageRequestMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "storageRequestMatcher matches on update"
	}
	return "storageRequestMatcher unknown context"
}

// SystemMatcher is a gomock.Matcher
type SystemMatcher struct {
	t                  *testing.T
	FetchParam         *system.SystemFetchParams
	UpdateParam        *system.SystemUpdateParams
	FetchHostnameParam *system.SystemHostnameFetchParams
}

// NewSystemMatcher returns a new matcher
func NewSystemMatcher(t *testing.T, param interface{}) *SystemMatcher {
	m := &SystemMatcher{t: t}
	switch p := param.(type) {
	case *system.SystemFetchParams:
		m.FetchParam = p
	case *system.SystemUpdateParams:
		m.UpdateParam = p
	case *system.SystemHostnameFetchParams:
		m.FetchHostnameParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *SystemMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *system.SystemFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *system.SystemUpdateParams:
		// // avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set, "uSet") &&
			assert.EqualValues(uRemove, p.Remove, "uRemove") &&
			assert.EqualValues(uAppend, p.Append, "uAppend") &&
			assert.EqualValues(o.UpdateParam.Version, p.Version, "Version") &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload, "Payload")
	case *system.SystemHostnameFetchParams:
		o.FetchHostnameParam.Context = p.Context
		return assert.EqualValues(o.FetchHostnameParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *SystemMatcher) String() string {
	if o.FetchParam != nil {
		return "systemMatcher matches on fetch"
	} else if o.UpdateParam != nil {
		return "systemMatcher matches on update"
	}
	return "systemMatcher unknown context"
}

// TaskMatcher is a gomock.Matcher
type TaskMatcher struct {
	t            *testing.T
	CreateParam  *task.TaskCreateParams
	FetchParam   *task.TaskFetchParams
	ListParam    *task.TaskListParams
	CancelParams *task.TaskCancelParams
}

// NewTaskMatcher returns a new matcher
func NewTaskMatcher(t *testing.T, param interface{}) *TaskMatcher {
	m := &TaskMatcher{t: t}
	switch p := param.(type) {
	case *task.TaskCreateParams:
		m.CreateParam = p
	case *task.TaskFetchParams:
		m.FetchParam = p
	case *task.TaskListParams:
		m.ListParam = p
	case *task.TaskCancelParams:
		m.CancelParams = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *TaskMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *task.TaskCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *task.TaskFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *task.TaskListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *task.TaskCancelParams:
		// avoid matching the context
		o.CancelParams.Context = p.Context
		return assert.EqualValues(o.CancelParams, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *TaskMatcher) String() string {
	if o.ListParam != nil {
		return "taskMatcher matches on list"
	}
	if o.FetchParam != nil {
		return "taskMatcher matches on fetch"
	}
	if o.CreateParam != nil {
		return "taskMatcher matches on create"
	}
	if o.CancelParams != nil {
		return "taskMatcher matches on cancel"
	}
	return "taskMatcher unknown context"
}

// UserMatcher is a gomock.Matcher
type UserMatcher struct {
	t           *testing.T
	CreateParam *user.UserCreateParams
	DeleteParam *user.UserDeleteParams
	FetchParam  *user.UserFetchParams
	ListParam   *user.UserListParams
	UpdateParam *user.UserUpdateParams
}

// NewUserMatcher returns a new matcher
func NewUserMatcher(t *testing.T, param interface{}) *UserMatcher {
	m := &UserMatcher{t: t}
	switch p := param.(type) {
	case *user.UserCreateParams:
		m.CreateParam = p
	case *user.UserDeleteParams:
		m.DeleteParam = p
	case *user.UserFetchParams:
		m.FetchParam = p
	case *user.UserListParams:
		m.ListParam = p
	case *user.UserUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *UserMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *user.UserCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *user.UserDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *user.UserFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *user.UserListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *user.UserUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *UserMatcher) String() string {
	if o.ListParam != nil {
		return "userMatcher matches on list"
	}
	return "userMatcher unknown context"
}

// VolumeSeriesMatcher is a gomock.Matcher
type VolumeSeriesMatcher struct {
	t           *testing.T
	CreateParam *volume_series.VolumeSeriesCreateParams
	DeleteParam *volume_series.VolumeSeriesDeleteParams
	FetchParam  *volume_series.VolumeSeriesFetchParams
	ListParam   *volume_series.VolumeSeriesListParams
	UpdateParam *volume_series.VolumeSeriesUpdateParams
}

// NewVolumeSeriesMatcher returns a new matcher
func NewVolumeSeriesMatcher(t *testing.T, param interface{}) *VolumeSeriesMatcher {
	m := &VolumeSeriesMatcher{t: t}
	switch p := param.(type) {
	case *volume_series.VolumeSeriesCreateParams:
		m.CreateParam = p
	case *volume_series.VolumeSeriesDeleteParams:
		m.DeleteParam = p
	case *volume_series.VolumeSeriesFetchParams:
		m.FetchParam = p
	case *volume_series.VolumeSeriesListParams:
		m.ListParam = p
	case *volume_series.VolumeSeriesUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *VolumeSeriesMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *volume_series.VolumeSeriesCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam, p)
	case *volume_series.VolumeSeriesDeleteParams:
		// avoid matching the context
		o.DeleteParam.Context = p.Context
		return assert.EqualValues(o.DeleteParam, p)
	case *volume_series.VolumeSeriesFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *volume_series.VolumeSeriesListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		return assert.EqualValues(o.ListParam, p)
	case *volume_series.VolumeSeriesUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *VolumeSeriesMatcher) String() string {
	if o.CreateParam != nil {
		return "volumeSeriesMatcher matches on create"
	} else if o.DeleteParam != nil {
		return "volumeSeriesMatcher matches on delete"
	} else if o.FetchParam != nil {
		return "volumeSeriesMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "volumeSeriesMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "volumeSeriesMatcher matches on update"
	}
	return "volumeSeriesMatcher unknown context"
}

// VolumeSeriesRequestMatcher is a gomock.Matcher
type VolumeSeriesRequestMatcher struct {
	t                       *testing.T
	CancelParam             *volume_series_request.VolumeSeriesRequestCancelParams
	CreateParam             *volume_series_request.VolumeSeriesRequestCreateParams
	FetchParam              *volume_series_request.VolumeSeriesRequestFetchParams
	ListParam               *volume_series_request.VolumeSeriesRequestListParams
	UpdateParam             *volume_series_request.VolumeSeriesRequestUpdateParams
	D                       time.Duration // Create CompleteByTime
	DActiveOrTimeModifiedGE time.Duration // List
	ZeroMsgTime             bool
}

// NewVolumeSeriesRequestMatcher returns a new matcher
func NewVolumeSeriesRequestMatcher(t *testing.T, param interface{}) *VolumeSeriesRequestMatcher {
	m := &VolumeSeriesRequestMatcher{t: t}
	switch p := param.(type) {
	case *volume_series_request.VolumeSeriesRequestCancelParams:
		m.CancelParam = p
	case *volume_series_request.VolumeSeriesRequestCreateParams:
		m.CreateParam = p
	case *volume_series_request.VolumeSeriesRequestFetchParams:
		m.FetchParam = p
	case *volume_series_request.VolumeSeriesRequestListParams:
		m.ListParam = p
	case *volume_series_request.VolumeSeriesRequestUpdateParams:
		m.UpdateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *VolumeSeriesRequestMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *volume_series_request.VolumeSeriesRequestCancelParams:
		// avoid matching the context
		o.CancelParam.Context = p.Context
		return assert.EqualValues(o.CancelParam, p)
	case *volume_series_request.VolumeSeriesRequestCreateParams:
		// avoid matching the context and time
		o.CreateParam.Context = p.Context
		cb := time.Time(p.Payload.CompleteByTime)
		p.Payload.CompleteByTime = o.CreateParam.Payload.CompleteByTime
		return assert.EqualValues(o.CreateParam, p) &&
			assert.True(time.Now().Add(o.D).After(cb))
	case *volume_series_request.VolumeSeriesRequestFetchParams:
		// avoid matching the context
		o.FetchParam.Context = p.Context
		return assert.EqualValues(o.FetchParam, p)
	case *volume_series_request.VolumeSeriesRequestListParams:
		// avoid matching the context
		o.ListParam.Context = p.Context
		var now = time.Now()
		var aTmGE time.Time
		if o.ListParam.ActiveOrTimeModifiedGE != nil {
			aTmGE = time.Time(*p.ActiveOrTimeModifiedGE)
			p.ActiveOrTimeModifiedGE = o.ListParam.ActiveOrTimeModifiedGE
		}
		return assert.EqualValues(o.ListParam, p) &&
			(o.ListParam.ActiveOrTimeModifiedGE == nil || assert.True(now.Add(-o.DActiveOrTimeModifiedGE).After(aTmGE)))
	case *volume_series_request.VolumeSeriesRequestUpdateParams:
		// avoid matching the context
		o.UpdateParam.Context = p.Context
		if o.ZeroMsgTime {
			mt := strfmt.DateTime(time.Now())
			for i := range p.Payload.RequestMessages {
				p.Payload.RequestMessages[i].Time = mt
			}
			for i := range o.UpdateParam.Payload.RequestMessages {
				o.UpdateParam.Payload.RequestMessages[i].Time = mt
			}
		}
		sort.Strings(p.Set)
		sort.Strings(p.Append)
		sort.Strings(p.Remove)
		uSet := o.UpdateParam.Set
		uAppend := o.UpdateParam.Append
		uRemove := o.UpdateParam.Remove
		sort.Strings(uSet)
		sort.Strings(uAppend)
		sort.Strings(uRemove)
		return assert.EqualValues(uSet, p.Set) &&
			assert.EqualValues(uRemove, p.Remove) &&
			assert.EqualValues(uAppend, p.Append) &&
			assert.EqualValues(o.UpdateParam.ID, p.ID) &&
			assert.EqualValues(o.UpdateParam.Version, p.Version) &&
			assert.EqualValues(o.UpdateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *VolumeSeriesRequestMatcher) String() string {
	if o.CreateParam != nil {
		return "volumeSeriesRequestMatcher matches on create"
	} else if o.FetchParam != nil {
		return "volumeSeriesRequestMatcher matches on fetch"
	} else if o.ListParam != nil {
		return "volumeSeriesRequestMatcher matches on list"
	} else if o.UpdateParam != nil {
		return "volumeSeriesRequestMatcher matches on update"
	} else if o.CancelParam != nil {
		return "volumeSeriesRequestMatcher matches on cancel"
	}
	return "volumeSeriesRequestMatcher unknown context"
}

// WatcherMatcher is a gomock.Matcher
type WatcherMatcher struct {
	t           *testing.T
	CreateParam *watchers.WatcherCreateParams
}

// NewWatcherMatcher returns a new matcher
func NewWatcherMatcher(t *testing.T, param interface{}) *WatcherMatcher {
	m := &WatcherMatcher{t: t}
	switch p := param.(type) {
	case *watchers.WatcherCreateParams:
		m.CreateParam = p
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *WatcherMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *watchers.WatcherCreateParams:
		// avoid matching the context
		o.CreateParam.Context = p.Context
		return assert.EqualValues(o.CreateParam.Payload, p.Payload)
	}
	return false
}

// String is from gomock.Matcher
func (o *WatcherMatcher) String() string {
	if o.CreateParam != nil {
		return "WatcherMatcher matches on create"
	}
	return "WatcherMatcher unknown context"
}
