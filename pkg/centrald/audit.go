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


package centrald

// AuditAction is an enumeration of the actions for an audit or event record
type AuditAction int

// AuditAction values; add entries to audit/handler.go actionMap for these values.
const (
	NoOpAuditAction AuditAction = iota

	AccountCreateAction
	AccountDeleteAction
	AccountUpdateAction

	ApplicationGroupCreateAction
	ApplicationGroupDeleteAction
	ApplicationGroupUpdateAction

	ClusterCreateAction
	ClusterDeleteAction
	ClusterUpdateAction

	ConsistencyGroupCreateAction
	ConsistencyGroupDeleteAction
	ConsistencyGroupUpdateAction

	CspCredentialCreateAction
	CspCredentialDeleteAction
	CspCredentialUpdateAction

	CspDomainCreateAction
	CspDomainDeleteAction
	CspDomainUpdateAction

	NodeCreateAction
	NodeDeleteAction
	NodeUpdateAction

	ProtectionDomainClearAction
	ProtectionDomainCreateAction
	ProtectionDomainDeleteAction
	ProtectionDomainSetAction
	ProtectionDomainUpdateAction

	ServicePlanAllocationCreateAction
	ServicePlanAllocationDeleteAction
	ServicePlanAllocationUpdateAction

	ServicePlanCloneAction
	ServicePlanDeleteAction
	ServicePlanUpdateAction

	UserCreateAction
	UserDeleteAction
	UserUpdateAction

	VolumeSeriesCreateAction
	VolumeSeriesDeleteAction
	VolumeSeriesUpdateAction

	LastAuditAction // must be last, for use in UT only
)
