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

all:

PKG=mock

# specify the interfaces that need to be mocked
MOCK_IFS=Exec,Cmd
MOCK_UTIL_PATH=github.com/Nuvoloso/kontroller/pkg/util
MOCK_OUTPUT=mock_exec.go

mock:: $(MOCK_OUTPUT)

$(MOCK_OUTPUT): ../exec.go Makefile
	@echo "* Mocking interfaces: $(MOCK_IFS)"
	mockgen -destination $(MOCK_OUTPUT) -package $(PKG) $(MOCK_UTIL_PATH) $(MOCK_IFS)
	@echo "**** $(MOCK_OUTPUT) should be checked in!"

unmock::
	$(RM) $(MOCK_OUTPUT)
