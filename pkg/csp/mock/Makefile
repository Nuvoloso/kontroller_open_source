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

MOCK_CSP_IFS=CloudServiceProvider,DomainClient
MOCK_CSP_PATH=github.com/Nuvoloso/kontroller/pkg/csp
MOCK_CSP=mock_csp.go

mock:: $(MOCK_CSP)

$(MOCK_CSP): ../csp.go
	@echo "* Mocking interfaces: $(MOCK_CSP_IFS)"
	mockgen -destination $(MOCK_CSP) -package $(PKG) $(MOCK_CSP_PATH) $(MOCK_CSP_IFS)
	@echo "**** $(MOCK_CSP) should be checked in!"

unmock::
	$(RM) $(MOCK_CSP)
