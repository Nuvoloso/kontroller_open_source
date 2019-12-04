# Makefile to build mock interfaces
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

PKG=mock

# specify the interfaces that need to be mocked
MOCK_MGMTCLIENT_IFS=API,AccountClient,ApplicationGroupClient,AuditLogClient,AuthenticationAPI,ClusterClient,ConsistencyGroupClient,CSPCredentialClient,CSPDomainClient,CSPStorageTypeClient,DebugClient,MetricsClient,NodeClient,PoolClient,ProtectionDomainClient,RoleClient,ServicePlanAllocationClient,ServicePlanClient,SLOClient,SnapshotClient,StorageClient,StorageFormulaClient,StorageRequestClient,SystemClient,TaskClient,UserClient,VolumeSeriesClient,VolumeSeriesRequestClient,WatchersClient
MOCK_MGMTCLIENT_PATH=github.com/Nuvoloso/kontroller/pkg/mgmtclient
MOCK_MGMTCLIENT=mock_mgmtclient.go

mock:: $(MOCK_MGMTCLIENT)

$(MOCK_MGMTCLIENT): ../api.go
	@echo "* Mocking interfaces: $(MOCK_MGMTCLIENT_IFS)"
	mockgen -destination $(MOCK_MGMTCLIENT) -package $(PKG) $(MOCK_MGMTCLIENT_PATH) $(MOCK_MGMTCLIENT_IFS)
	@echo "**** $(MOCK_MGMTCLIENT) should be checked in!"

unmock::
	$(RM) $(MOCK_MGMTCLIENT)
