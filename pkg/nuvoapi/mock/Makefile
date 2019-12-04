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

# specify the nuvoapi interfaces that need to be mocked
MOCK_NUVOAPI_IFS=NuvoVM
MOCK_NUVOAPI_PATH=github.com/Nuvoloso/kontroller/pkg/nuvoapi
MOCK_NUVOAPI=mock_nuvoapi.go

mock:: $(MOCK_NUVOAPI)

$(MOCK_NUVOAPI): ../nuvoapi.go Makefile
	@echo "* Mocking interfaces: $(MOCK_NUVOAPI_IFS)"
	mockgen -destination $(MOCK_NUVOAPI) -package $(PKG) $(MOCK_NUVOAPI_PATH) $(MOCK_NUVOAPI_IFS)
	@echo "**** $(MOCK_NUVOAPI) should be checked in!"

unmock::
	$(RM) $(MOCK_NUVOAPI)
