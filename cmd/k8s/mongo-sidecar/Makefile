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


.PHONY: all build clean veryclean

all: build

CP=cp
MKDIR=mkdir -p

PROGRAM=nv-mongo-sidecar
BIN_DIR=../../../deploy/bin

BUILD_TIME := $(shell date +%FT%T%z)
BUILD_ID := $(shell git rev-parse HEAD)
BUILD_HOST := $(shell hostname)

# BUILD_JOB is set by jenkins_build.sh
BUILD_LDFLAGS=-ldflags="-X 'main.BuildTime=$(BUILD_TIME)' -X 'main.BuildID=$(BUILD_ID)' -X 'main.BuildHost=$(BUILD_HOST)' -X 'main.BuildJob=$(BUILD_JOB)' -X 'main.Appname=$(PROGRAM)'"
BUILD_FLAGS=$(BUILD_LDFLAGS) $(BUILD_EXTRA_FLAGS)

build::
	go build $(BUILD_FLAGS) -o $(PROGRAM)
	$(MKDIR) $(BIN_DIR)
	$(CP) $(PROGRAM) $(BIN_DIR)

clean::
	go clean
	$(RM) $(PROGRAM)

veryclean:: clean
