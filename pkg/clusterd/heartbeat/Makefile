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

# specify the clusterd interfaces that need to be mocked
MOCK_CLUSTERD_IFS=AppServant
MOCK_CLUSTERD_PATH=github.com/Nuvoloso/kontroller/pkg/clusterd
MOCK_CLUSTERD=mock_clusterd_test.go

mock:: $(MOCK_CLUSTERD)

$(MOCK_CLUSTERD): ../appctx.go Makefile
	@echo "* Mocking mgmtclient interfaces: $(MOCK_CLUSTERD_IFS)"
	mockgen -destination $(MOCK_CLUSTERD) -package heartbeat -write_package_comment=false $(MOCK_CLUSTERD_PATH) $(MOCK_CLUSTERD_IFS)
	@echo "**** Adjusting for vendor imports (https://github.com/golang/go/issues/14566)"
	sed -e 's;github.com/Nuvoloso/kontroller/vendor/;;' $(MOCK_CLUSTERD) > $(MOCK_CLUSTERD).tmp
	mv $(MOCK_CLUSTERD).tmp $(MOCK_CLUSTERD)
	@echo "**** $(MOCK_CLUSTERD) should be checked in!"

unmock::
	$(RM) $(MOCK_CLUSTERD)
