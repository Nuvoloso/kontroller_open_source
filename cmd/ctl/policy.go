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


package main

import (
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// CUPFlags has common flags for the ClusterUsagePolicy
type CUPFlags struct {
	CupAccountSecretScope          string `short:"S" long:"cup-account-secret-scope" description:"Set the scope for account secrets in a local cluster usage policy" choice:"CLUSTER" choice:"CSPDOMAIN" choice:"GLOBAL"`
	CupConsistencyGroupName        string `long:"cup-consistency-group-name" description:"Specify the consistency group name template in a local cluster usage policy"`
	CupVolumeDataRetentionOnDelete string `long:"cup-data-retention-on-delete" description:"Specifies in a local cluster usage policy how data is handled in Nuvo when the associated cluster volume is deleted" choice:"DELETE" choice:"RETAIN"`
}

// CupProcessLocalFlags will modify the given CUP against the flags.
func (c *CUPFlags) CupProcessLocalFlags(cup *models.ClusterUsagePolicy) bool {
	if c.CupAccountSecretScope != "" || c.CupConsistencyGroupName != "" || c.CupVolumeDataRetentionOnDelete != "" {
		cup.Inherited = false
		if c.CupAccountSecretScope != "" {
			cup.AccountSecretScope = c.CupAccountSecretScope
		}
		if c.CupConsistencyGroupName != "" {
			cup.ConsistencyGroupName = c.CupConsistencyGroupName
		}
		if c.CupVolumeDataRetentionOnDelete != "" {
			cup.VolumeDataRetentionOnDelete = c.CupVolumeDataRetentionOnDelete
		}
		return true
	}
	return false
}

// CUPFlagsWithInheritance has common flags for inheritable ClusterUsagePolicy
type CUPFlagsWithInheritance struct {
	CUPFlags
	InheritClusterUsagePolicy bool `long:"cup-inherit" description:"Inherit the cluster usage policy"`
}

// CupValidateInheritableFlags validates inheritable flag combinations
func (c *CUPFlagsWithInheritance) CupValidateInheritableFlags() error {
	if c.InheritClusterUsagePolicy && (c.CupAccountSecretScope != "" || c.CupVolumeDataRetentionOnDelete != "") {
		return fmt.Errorf("cannot both set local values and inherit cluster usage policy")
	}
	return nil
}

// CupProcessInheritableFlags will modify the given CUP against the flags.
func (c *CUPFlagsWithInheritance) CupProcessInheritableFlags(cup *models.ClusterUsagePolicy) bool {
	if c.InheritClusterUsagePolicy {
		newCup := &models.ClusterUsagePolicy{Inherited: true}
		*cup = *newCup
		return true
	}
	return c.CupProcessLocalFlags(cup)
}

// SMPFlags has common flags for the SnapshotManagementPolicy
type SMPFlags struct {
	SmpDisableSnapshots            bool   `long:"smp-disable-snapshots" description:"Snapshots of volumes in this consistency group are disabled if set"`
	SmpEnableSnapshots             bool   `long:"smp-enable-snapshots" description:"Snapshots of volumes in this consistency group are enabled if set"`
	SmpDeleteSnapshots             bool   `long:"smp-delete-snapshots" description:"Deletion of snapshots of volumes in this consistency group is enabled if set"`
	SmpNoDeleteSnapshots           bool   `long:"smp-no-delete-snapshots" description:"Deletion of snapshots of volumes in this consistency group is disabled if set"`
	SmpDeleteLast                  bool   `long:"smp-delete-last-snapshot" description:"Deletion of the last snapshot of volume in this consistency group is enabled if set"`
	SmpNoDeleteLast                bool   `long:"smp-no-delete-last-snapshot" description:"Deletion of the last snapshot of volume in this consistency group is disabled if set"`
	SmpDeleteVolumeWithLast        bool   `long:"smp-delete-vol-with-last" description:"Deletion of the volume when its last snapshot gets deleted is enabled if set"`
	SmpNoDeleteVolumeWithLast      bool   `long:"smp-no-delete-vol-with-last" description:"Deletion of the volume when its last snapshot gets deleted is disabled if set"`
	SmpRetentionPeriodDays         int32  `long:"smp-retention-days" description:"Number of days to retain snapshot objects"`
	SmpVolumeDataRetentionOnDelete string `long:"smp-data-retention-on-delete" description:"Specifies how data is handled in Nuvo when the associated cluster volume is deleted" choice:"DELETE" choice:"RETAIN"`
}

// SmpValidateFlags validates the flags
func (c *SMPFlags) SmpValidateFlags() error {
	if (c.SmpDisableSnapshots && c.SmpEnableSnapshots) || (c.SmpDeleteSnapshots && c.SmpNoDeleteSnapshots) || (c.SmpDeleteLast && c.SmpNoDeleteLast) || (c.SmpDeleteVolumeWithLast && c.SmpNoDeleteVolumeWithLast) {
		return fmt.Errorf("do not specify 'smp-enable-snapshots' and 'smp-disable-snapshots' or 'smp-delete-snapshots' and 'smp-no-delete-snapshots' together")
	}
	if c.SmpNoDeleteSnapshots && c.SmpRetentionPeriodDays != 0 {
		return fmt.Errorf("do not specify 'smp-no-delete-snapshots' and 'smp-retention-days' together")
	}
	return nil
}

// SmpProcessLocalFlags will modify the given SMP against the flags.
func (c *SMPFlags) SmpProcessLocalFlags(smp *models.SnapshotManagementPolicy) bool {
	changed := false
	if c.SmpDisableSnapshots && !smp.DisableSnapshotCreation {
		smp.DisableSnapshotCreation = true
		changed = true
	} else if c.SmpEnableSnapshots && smp.DisableSnapshotCreation {
		smp.DisableSnapshotCreation = false
		changed = true
	}
	if c.SmpDeleteSnapshots && smp.NoDelete {
		smp.NoDelete = false
		changed = true
	} else if c.SmpNoDeleteSnapshots && !smp.NoDelete {
		smp.NoDelete = true
		changed = true
	}
	if c.SmpDeleteLast && !smp.DeleteLast {
		smp.DeleteLast = true
		changed = true
	} else if c.SmpNoDeleteLast && smp.DeleteLast {
		smp.DeleteLast = false
		changed = true
	}
	if c.SmpDeleteVolumeWithLast && !smp.DeleteVolumeWithLast {
		smp.DeleteVolumeWithLast = true
		changed = true
	} else if c.SmpNoDeleteVolumeWithLast && smp.DeleteVolumeWithLast {
		smp.DeleteVolumeWithLast = false
		changed = true
	}
	if c.SmpRetentionPeriodDays != 0 && swag.Int32Value(smp.RetentionDurationSeconds) != c.SmpRetentionPeriodDays {
		smp.RetentionDurationSeconds = swag.Int32(c.SmpRetentionPeriodDays * 24 * 60 * 60)
		changed = true
	}
	if c.SmpVolumeDataRetentionOnDelete != "" && smp.VolumeDataRetentionOnDelete != c.SmpVolumeDataRetentionOnDelete {
		smp.VolumeDataRetentionOnDelete = c.SmpVolumeDataRetentionOnDelete
		changed = true
	}
	if changed {
		smp.Inherited = false
	}
	return changed
}

// SMPFlagsWithInheritance has common flags for inheritable SnapshotManagementPolicy
type SMPFlagsWithInheritance struct {
	SMPFlags
	InheritSnapshotManagementPolicy bool `long:"smp-inherit" description:"Inherit the snapshot management policy"`
}

// SmpValidateInheritableFlags validates inheritable flag combinations
func (c *SMPFlagsWithInheritance) SmpValidateInheritableFlags() error {
	if c.InheritSnapshotManagementPolicy && (c.SmpEnableSnapshots || c.SmpDisableSnapshots || c.SmpDeleteSnapshots || c.SmpNoDeleteSnapshots || c.SmpDeleteLast || c.SmpNoDeleteLast || c.SmpDeleteVolumeWithLast || c.SmpNoDeleteVolumeWithLast) {
		return fmt.Errorf("do not specify 'smp-inherit' together with modifications to the snapshot management policy")
	}
	return c.SmpValidateFlags()
}

// SmpProcessInheritableFlags will modify the given SMP against the flags.
func (c *SMPFlagsWithInheritance) SmpProcessInheritableFlags(smp *models.SnapshotManagementPolicy) bool {
	if c.InheritSnapshotManagementPolicy {
		if smp.Inherited {
			return false
		}
		newSmp := &models.SnapshotManagementPolicy{Inherited: true}
		*smp = *newSmp
		return true
	}
	return c.SmpProcessLocalFlags(smp)
}

// VSRPFlags has common flags for the VSR ManagementPolicy
type VSRPFlags struct {
	VsrpRetentionPeriodDays int32 `long:"vsrp-retention-days" description:"Number of days to retain VSR objects"`
}

// VsrpProcessLocalFlags will modify the given VSR policy against the flags.
func (c *VSRPFlags) VsrpProcessLocalFlags(vsrp *models.VsrManagementPolicy) bool {
	if c.VsrpRetentionPeriodDays != 0 {
		vsrp.Inherited = false
		if c.VsrpRetentionPeriodDays != 0 {
			vsrp.RetentionDurationSeconds = swag.Int32(c.VsrpRetentionPeriodDays * 24 * 60 * 60)
		}
		return true
	}
	return false
}

// VSRPFlagsWithInheritance has common flags for inheritable VSR ManagementPolicy
type VSRPFlagsWithInheritance struct {
	VSRPFlags
	InheritVSRManagementPolicy bool `long:"vsrp-inherit" description:"Inherit the VSR management policy"`
}

// VsrpValidateInheritableFlags validates inheritable flag combinations
func (c *VSRPFlagsWithInheritance) VsrpValidateInheritableFlags() error {
	if c.InheritVSRManagementPolicy && c.VsrpRetentionPeriodDays != 0 {
		return fmt.Errorf("do not specify 'vsrp-inherit' together with modifications to the VSR management policy")
	}
	return nil
}

// VsrpProcessInheritableFlags will modify the given VSR policy against the flags.
func (c *VSRPFlagsWithInheritance) VsrpProcessInheritableFlags(vsrp *models.VsrManagementPolicy) bool {
	if c.InheritVSRManagementPolicy {
		if vsrp.Inherited {
			return false
		}
		newVsrp := &models.VsrManagementPolicy{Inherited: true}
		*vsrp = *newVsrp
		return true
	}
	return c.VsrpProcessLocalFlags(vsrp)
}

// SCPFlags has common flags for the SnapshotCatalogPolicy
type SCPFlags struct {
	ScpCspDomain          string `long:"scp-domain" description:"The name of the CSPDomain object representing the protection store. Required if id not specified"`
	ScpCspDomainID        string `long:"scp-domain-id" description:"The ID of the CSPDomain object representing the protection store"`
	ScpProtectionDomain   string `long:"scp-protection-domain" description:"The name of the ProtectionDomain used to secure the snapshot metadata in the protection store. Required if id not specified"`
	ScpProtectionDomainID string `long:"scp-protection-domain-id" description:"The ID of the ProtectionDomain used to secure the snapshot metadata in the protection store."`
}

// ScpProcessLocalFlags will modify the given SCP against the flags.
func (c *SCPFlags) ScpProcessLocalFlags(scp *models.SnapshotCatalogPolicy) bool {
	if c.ScpCspDomainID != "" || c.ScpProtectionDomainID != "" {
		scp.Inherited = false
		if c.ScpCspDomainID != "" {
			scp.CspDomainID = models.ObjIDMutable(c.ScpCspDomainID)
		}
		if c.ScpProtectionDomainID != "" {
			scp.ProtectionDomainID = models.ObjIDMutable(c.ScpProtectionDomainID)
		}
		return true
	}
	return false
}

// SCPFlagsWithInheritance has common flags for inheritable SnapshotCatalogPolicy
type SCPFlagsWithInheritance struct {
	SCPFlags
	InheritSnapshotCatalogPolicy bool `long:"scp-inherit" description:"Inherit the snapshot catalog policy"`
}

// ScpValidateInheritableFlags validates inheritable flag combinations
func (c *SCPFlagsWithInheritance) ScpValidateInheritableFlags() error {
	if c.InheritSnapshotCatalogPolicy && (c.ScpCspDomain != "" || c.ScpCspDomainID != "" || c.ScpProtectionDomain != "" || c.ScpProtectionDomainID != "") {
		return fmt.Errorf("do not specify 'scp-inherit' together with modifications to the snapshot catalog policy")
	}
	return nil
}

// ScpProcessInheritableFlags will modify the given SnapshotCatalogPolicy against the flags.
func (c *SCPFlagsWithInheritance) ScpProcessInheritableFlags(scp *models.SnapshotCatalogPolicy) bool {
	if c.InheritSnapshotCatalogPolicy {
		if scp.Inherited {
			return false
		}
		newScp := &models.SnapshotCatalogPolicy{Inherited: true}
		*scp = *newScp
		return true
	}
	return c.ScpProcessLocalFlags(scp)
}
