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

MOCK_DB_IFS=Client,ObjectDocumentHandler
MOCK_DB_PATH=github.com/Nuvoloso/kontroller/pkg/mongodb
MOCK_DB=mock_db_test.go

all:
	echo "*** You must explicitly make mock to rebuild mocks ***" 2>&1
	@exit 1

mock:: $(MOCK_DB)

$(MOCK_DB): interface.go Makefile
	mockgen -destination $(MOCK_DB) -package mongodb -self_package $(MOCK_DB_PATH) -write_package_comment=false $(MOCK_DB_PATH) $(MOCK_DB_IFS)
	@echo "**** $(MOCK_DB) should be checked in!"

unmock:
	$(RM) $(MOCK_DB)
