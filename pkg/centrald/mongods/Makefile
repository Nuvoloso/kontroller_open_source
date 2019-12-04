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

MOCK_DS_IFS=DBAPI,ObjectDocumentHandler,ObjectDocumentHandlerCRUD
MOCK_DS_PATH=github.com/Nuvoloso/kontroller/pkg/centrald/mongods
MOCK_DS=mock_ds_test.go

all:
	echo "*** You must explicitly make mock to rebuild mocks ***" 2>&1
	@exit 1

mock:: $(MOCK_DS)

$(MOCK_DS): dbintf.go odh.go Makefile
	mockgen -destination $(MOCK_DS) -package mongods -self_package $(MOCK_DS_PATH) -write_package_comment=false $(MOCK_DS_PATH) $(MOCK_DS_IFS)
	@echo "**** $(MOCK_DS) should be checked in!"

unmock:
	$(RM) $(MOCK_DS)
