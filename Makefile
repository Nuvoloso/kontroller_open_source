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


.PHONY: all build depend generate jsonlint clean veryclean test

all: build

BUILD_DIRS=\
	cmd/agentd \
	cmd/aws \
	cmd/auth \
	cmd/azure \
	cmd/catalog \
	cmd/centrald \
	cmd/clmd \
	cmd/clusterd \
	cmd/ctl \
	cmd/copy \
	cmd/gcp \
	cmd/imd \
	cmd/k8s/mongo-sidecar \
	cmd/k8s/nuvo_fv \
	cmd/mdDecode \
	cmd/mdEncode \
	cmd/mountfs \
	cmd/nuvo_vm

build:: depend generate jsonlint copyrights
	@for d in $(BUILD_DIRS); do echo "** Build directory $$d"; $(MAKE) -C $$d build || exit 1; done

clean::
	for d in $(BUILD_DIRS); do echo "*** Clean in $$d "; $(MAKE) -C $$d clean; done

TEST_DIRS=\
  cmd/auth \
  cmd/copy \
  cmd/ctl \
  cmd/k8s/mongo-sidecar \
  cmd/k8s/nuvo_fv \
  pkg/agentd \
  pkg/agentd/csi \
  pkg/agentd/handlers \
  pkg/agentd/heartbeat \
  pkg/agentd/metrics \
  pkg/agentd/state \
  pkg/agentd/sreq \
  pkg/agentd/vreq \
  pkg/auth \
  pkg/azuresdk \
  pkg/centrald \
  pkg/centrald/audit \
  pkg/centrald/auth \
  pkg/centrald/handlers \
  pkg/centrald/heartbeat \
  pkg/centrald/metrics \
  pkg/centrald/mongods \
  pkg/centrald/sreq \
  pkg/centrald/vreq \
  pkg/cluster \
  pkg/clusterd \
  pkg/clusterd/csi \
  pkg/clusterd/handlers \
  pkg/clusterd/heartbeat \
  pkg/clusterd/snapper \
  pkg/clusterd/state \
  pkg/clusterd/unbinder \
  pkg/clusterd/vreq \
  pkg/csi \
  pkg/csp \
  pkg/csp/aws \
  pkg/csp/azure \
  pkg/csp/gc \
  pkg/crude \
  pkg/encrypt \
  pkg/gcsdk \
  pkg/housekeeping \
  pkg/layout \
  pkg/metadata \
  pkg/metricmover \
  pkg/mgmtclient \
  pkg/mgmtclient/crud \
  pkg/mongodb \
  pkg/mount \
  pkg/nuvoapi \
  pkg/pgdb \
  pkg/pstore \
  pkg/rei \
  pkg/stats \
  pkg/testutils \
  pkg/util \
  pkg/vra \
  pkg/ws

TEST_FLAGS=-coverprofile cover.out -timeout 30s
EXTRA_TEST_FLAGS=
test:: generate
	@TOP=$$(pwd); for d in $(TEST_DIRS); do cd $$TOP/$$d; echo "** Test directory $$d"; go test $(TEST_FLAGS) $(EXTRA_TEST_FLAGS) || exit 1 ; done
	@echo Use \"go tool cover -html=cover.out -o=cover.html\" to view output in each directory

jsonlint::
	find deploy -name '*.json' | xargs ./jsonlint.py

copyrights:
	./copyrights.py

# auto generate Swagger server and client stubs
SWAGGER=swagger
SWAGGER_SPEC=api/swagger.yaml
SWAGGER_SERVER_CONFIG=api/goswagger/server_config.yaml
SWAGGER_CLIENT_CONFIG=api/goswagger/client_config.yaml
SWAGGER_APP=nuvolosoAPI
SWAGGER_AUTOGEN_DIR=pkg/autogen
SWAGGER_TEMPLATE_DIR := api/goswagger
SWAGGER_TEMPLATES := $(shell ls $(SWAGGER_TEMPLATE_DIR)/*.gotmpl)
AUTO_TGT=.autogen
$(AUTO_TGT): $(SWAGGER_SPEC) $(SWAGGER_CONFIG) $(SWAGGER_TEMPLATES)
	$(RM) -r $(SWAGGER_AUTOGEN_DIR)
	mkdir -p $(SWAGGER_AUTOGEN_DIR)
	$(SWAGGER) generate server -f $(SWAGGER_SPEC) --exclude-main --with-flatten=full -C $(SWAGGER_SERVER_CONFIG) -T . -A $(SWAGGER_APP) -t $(SWAGGER_AUTOGEN_DIR)
	$(SWAGGER) generate client -f $(SWAGGER_SPEC) --with-flatten=full -C $(SWAGGER_CLIENT_CONFIG) -T . -A $(SWAGGER_APP) -t $(SWAGGER_AUTOGEN_DIR) --skip-models
	./customize_models.sh
	touch $(AUTO_TGT)
generate:: $(AUTO_TGT)

clean::
	$(RM) $(AUTO_TGT)
	$(RM) -r $(SWAGGER_AUTOGEN_DIR)

depend::
veryclean:: clean

# This target serves pretty looking documentation but no ability to create a curl command
# or directly view the models.
docs:
	$(SWAGGER) serve $(SWAGGER_SPEC)

# The default doc target is the classic swagger document with curl command construction.
docs_classic:
	$(SWAGGER) serve -F swagger $(SWAGGER_SPEC)

# Launch a godoc server
godocs:
	godoc -http=:6060 -server .

validate::
	$(SWAGGER) validate $(SWAGGER_SPEC)

generate:: build_protobuf

# Build .go from .proto
PROTOBUF_DIR=pkg/nuvoapi/nuvo_pb/ pkg/csi/csi_pb/
build_protobuf:
	@for d in $(PROTOBUF_DIR); do echo "*** Build protobuf in $$d "; $(MAKE) -C $$d build || exit 1; done

clean::
	@for d in $(PROTOBUF_DIR); do echo "*** Clean in $$d "; $(MAKE) -C $$d clean; done

# CONTAINER_TAG can be specified on the command line or in the environment to override this default
CONTAINER_TAG?=latest
container_build:
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvcentrald:$(CONTAINER_TAG) --file=deploy/Dockerfile.nvcentrald .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvclusterd:$(CONTAINER_TAG) --file=deploy/Dockerfile.nvclusterd .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvagentd:$(CONTAINER_TAG) --file=deploy/Dockerfile.nvagentd .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvauth:$(CONTAINER_TAG) --file=deploy/Dockerfile.nvauth .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nv-mongo-sidecar:$(CONTAINER_TAG) --file=deploy/Dockerfile.nv-mongo-sidecar .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/configdb:$(CONTAINER_TAG) --file=deploy/Dockerfile.configdb .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/metricsdb:$(CONTAINER_TAG) --file=deploy/Dockerfile.metricsdb .
	docker build -t 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nuvo_fv:$(CONTAINER_TAG) --file=deploy/Dockerfile.nuvo_fv .

container_push:
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvcentrald:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvclusterd:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvagentd:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nvauth:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nv-mongo-sidecar:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/configdb:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/metricsdb:$(CONTAINER_TAG)
	docker push 407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/nuvo_fv:$(CONTAINER_TAG)

container: container_build container_push

container_metricsdb:
	docker build -t metricsdb:$(CONTAINER_TAG) --file=deploy/Dockerfile.metricsdb .
