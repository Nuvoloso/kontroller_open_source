# Makefile to build the heartbeat mock interfaces
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

# specify the agentd interfaces that need to be mocked
MOCK_AGENTD_IFS=AppServant
MOCK_AGENTD_PATH=github.com/Nuvoloso/kontroller/pkg/agentd
MOCK_AGENTD=mock_agentd_test.go

mock:: $(MOCK_AGENTD)

$(MOCK_AGENTD): ../appctx.go Makefile
	@echo "* Mocking mgmtclient interfaces: $(MOCK_AGENTD_IFS)"
	mockgen -destination $(MOCK_AGENTD) -package heartbeat -write_package_comment=false $(MOCK_AGENTD_PATH) $(MOCK_AGENTD_IFS)
	@echo "**** Adjusting for vendor imports (https://github.com/golang/go/issues/14566)"
	sed -e 's;github.com/Nuvoloso/kontroller/vendor/;;' $(MOCK_AGENTD) > $(MOCK_AGENTD).tmp
	mv $(MOCK_AGENTD).tmp $(MOCK_AGENTD)
	@echo "**** $(MOCK_AGENTD) should be checked in!"

unmock::
	$(RM) $(MOCK_AGENTD)
