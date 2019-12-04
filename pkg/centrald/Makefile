# Copyright 2019 Tad Lebeck
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# Makefile to build the mock interfaces of the centrald package

.PHONY: all mock unmock

all::

MOCK_CENTRALD_IFS=DataStore,AccountOps,ApplicationGroupOps,ClusterOps,ConsistencyGroupOps,CspCredentialOps,CspDomainOps,NodeOps,PoolOps,ProtectionDomainOps,RoleOps,ServicePlanOps,ServicePlanAllocationOps,SnapshotOps,StorageOps,StorageRequestOps,SystemOps,UserOps,VolumeSeriesOps,VolumeSeriesRequestOps
MOCK_CENTRALD_PATH=github.com/Nuvoloso/kontroller/pkg/centrald
MOCK_CENTRALD=mock_centrald_test.go

mock:: $(MOCK_CENTRALD)

$(MOCK_CENTRALD): datastore.go Makefile
	mockgen -destination $(MOCK_CENTRALD) -package centrald -self_package $(MOCK_CENTRALD_PATH) -write_package_comment=false $(MOCK_CENTRALD_PATH) $(MOCK_CENTRALD_IFS)
	@echo "**** $(MOCK_CENTRALD) should be checked in!"

unmock::
	$(RM) $(MOCK_CENTRALD)
