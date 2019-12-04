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


.PHONY: all mock unmock

all:
	echo "*** You must explicitly make mock to rebuild mocks ***" 2>&1
	@exit 1

PKG=mock

# specify the interfaces that need to be mocked
MOCK_CENTRALD_IFS=AppCloudServiceProvider,AppCrudHelpers,DataStore,AccountOps,ApplicationGroupOps,ClusterOps,ConsistencyGroupOps,CspCredentialOps,CspDomainOps,NodeOps,PoolOps,ProtectionDomainOps,RoleOps,ServicePlanOps,ServicePlanAllocationOps,SnapshotOps,StorageOps,StorageRequestOps,SystemOps,UserOps,VolumeSeriesOps,VolumeSeriesRequestOps,StorageFormulaComputer
MOCK_CENTRALD_PATH=github.com/Nuvoloso/kontroller/pkg/centrald
MOCK_CENTRALD=mock_centrald.go

mock:: $(MOCK_CENTRALD)

$(MOCK_CENTRALD): ../datastore.go ../csp_domains.go Makefile
	@echo "* Mocking interfaces: $(MOCK_CENTRALD_IFS)"
	mockgen -destination $(MOCK_CENTRALD) -package $(PKG) $(MOCK_CENTRALD_PATH) $(MOCK_CENTRALD_IFS)
	@echo "**** $(MOCK_CENTRALD) should be checked in!"

unmock::
	$(RM) $(MOCK_CENTRALD)
