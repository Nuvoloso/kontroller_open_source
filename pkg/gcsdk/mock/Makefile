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


# Makefile to build mock interfaces


.PHONY: all mock unmock

all::

PKG=mock

# specify the interfaces that need to be mocked
MOCK_GC_SDK_IFS=API,ComputeService,DisksService,DisksDeleteCall,DisksGetCall,DisksInsertCall,DisksListCall,DisksSetLabelsCall,InstancesService,InstancesAttachDiskCall,InstancesDetachDiskCall,ZoneOperationsService,ZoneOperationsGetCall,ZonesService,ZonesGetCall,StorageClient,BucketHandle
MOCK_GC_SDK_PATH=github.com/Nuvoloso/kontroller/pkg/gcsdk
MOCK_GC_SDK=mock_gc.go

mock:: $(MOCK_GC_SDK)

$(MOCK_GC_SDK): ../api.go Makefile
	@echo "* Mocking interfaces: $(MOCK_GC_SDK_IFS)"
	mockgen -destination $(MOCK_GC_SDK) -package $(PKG) $(MOCK_GC_SDK_PATH) $(MOCK_GC_SDK_IFS)
	@echo "**** $(MOCK_GC_SDK) should be checked in!"

unmock::
	$(RM) $(MOCK_GC_SDK)
