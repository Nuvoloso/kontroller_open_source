# Makefile to generate go from protobuf
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


.PHONY: all build clean

all: build

PROTO_SRC=nuvo.proto

build:: nuvo.pb.go

nuvo.pb.go:: $(PROTO_SRC)
	protoc --proto_path=. --go_out=. $(PROTO_SRC)
	sed -e '1s/protoc-gen-go.$$/protoc-gen-go. DO NOT EDIT./' nuvo.pb.go > nuvo.pb.go.out && mv nuvo.pb.go.out nuvo.pb.go

clean::
	go clean
	$(RM) *.go
