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

import com "github.com/Nuvoloso/kontroller/pkg/common"

// Well defined names
const (
	// DefSubAccount is the name of the built-in normal account, subordinate to the built-in tenant account
	DefSubAccount = com.DefSubAccount
	// DefSubAccountDescription
	DefSubAccountDescription = "This is a built-in account subordinate to the built-in tenant account. It can be renamed, disabled or deleted."
	// DefTenantAccount is the name of the built-in tenant account
	DefTenantAccount = com.DefTenantAccount
	// DefTenantDescription
	DefTenantDescription = "This is the built-in tenant account. It can be renamed, disabled or deleted."
	// SystemAccount is the name of the administrative account
	SystemAccount = com.SystemAccount
	// SystemAccountDescription
	SystemAccountDescription = "This account is reserved for administrative operations"
	// SystemUser is the name of the default administrative user
	SystemUser = com.AdminUser
	// SystemAdminRole is the name of the uber administrative role
	SystemAdminRole = com.SystemAdminRole
	// TenantAdminRole is the name of the tenant administrative role
	TenantAdminRole = com.TenantAdminRole
	// AccountAdminRole is the name of the account administrative role
	AccountAdminRole = com.AccountAdminRole
	// AccountUserRole is the name of the account user role
	AccountUserRole = com.AccountUserRole
)

// Capabilities
const (
	AccountFetchAllRolesCap       = "accountFetchAllRoles"
	AccountFetchOwnRoleCap        = "accountFetchOwnRole"
	AccountUpdateCap              = "accountUpdate"
	AuditLogAnnotateCap           = "auditLogAnnotate"
	CSPDomainManagementCap        = "cspDomainManagement"
	CSPDomainUsageCap             = "cspDomainUsage"
	ManageNormalAccountsCap       = "manageNormalAccounts"
	ManageSpecialAccountsCap      = "manageSpecialAccounts"
	ProtectionDomainManagementCap = "protectionDomainManagement"
	SystemManagementCap           = "systemManagement"
	UserManagementCap             = "userManagement"
	VolumeSeriesFetchCap          = "volumeSeriesFetch"
	VolumeSeriesOwnerCap          = "volumeSeriesOwner"
)
