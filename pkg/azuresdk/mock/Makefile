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

all::

PKG=mock

MOCK_AZ_SDK_IFS=API,CCC,SPT,GC,AC,BCC,DC,DCCreateOrUpdateFuture,DCDisksDeleteFuture,DCListIterator,DCDisksUpdateFuture,VMC,VMCreateOrUpdateFuture
MOCK_AZ_SDK_PATH=github.com/Nuvoloso/kontroller/pkg/azuresdk
MOCK_AZ_SDK=mock_azuresdk.go

mock:: $(MOCK_AZ_SDK)

$(MOCK_AZ_SDK): ../api.go Makefile
	@echo "* Mocking interfaces: $(MOCK_AZ_SDK_IFS)"
	mockgen -destination $(MOCK_AZ_SDK) -package $(PKG) $(MOCK_AZ_SDK_PATH) $(MOCK_AZ_SDK_IFS)
	@echo "**** $(MOCK_AZ_SDK) should be checked in!"

unmock::
	$(RM) $(MOCK_AZ_SDK)
