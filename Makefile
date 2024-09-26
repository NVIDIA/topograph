# Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

LINTER_BIN ?= golangci-lint
DOCKER_BIN ?= docker
TARGETS := topograph node-observer toposim
CMD_DIR := ./cmd
OUTPUT_DIR := ./bin

IMAGE_REPO ?=docker.io/nvidia/topograph
GIT_REF =$(shell git rev-parse --abbrev-ref HEAD)
IMAGE_TAG ?=$(GIT_REF)

.PHONY: build
build: proto
	@for target in $(TARGETS); do        \
	  echo "Building $${target}";        \
	  CGO_ENABLED=0 go build -a -o $(OUTPUT_DIR)/$${target}        \
	    -ldflags '-extldflags "-static" -X main.GitTag=$(GIT_REF)' \
	    $(CMD_DIR)/$${target};           \
	done

.PHONY: clean
clean:
	scripts/clean-build.sh

.PHONY: test
test:
	@echo running tests
	go test -coverprofile=coverage.out -covermode=atomic -race ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	$(LINTER_BIN) run --new-from-rev "HEAD~$(git rev-list master.. --count)" ./...

.PHONY: mod
mod:
	go mod tidy

.PHONY: coverage
coverage: test
	go tool cover -func=coverage.out

.PHONY: init-proto
init-proto:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: proto
proto:
	protoc --proto_path=protos \
		--go_out=pkg/protos --go_opt=paths=source_relative \
		--go-grpc_out=pkg/protos --go-grpc_opt=paths=source_relative \
		protos/*.proto

.PHONY: image-build
image-build: build
	$(DOCKER_BIN) build -t $(IMAGE_REPO):$(IMAGE_TAG) -f ./Dockerfile .

.PHONY: image-push
image-push: image-build
	$(DOCKER_BIN) push $(IMAGE_REPO):$(IMAGE_TAG)

.PHONY: ssl
ssl:
	SSL_DIR=ssl ./scripts/configure-ssl.sh

.PHONY: deb
deb: build
	scripts/build-deb.sh $(GIT_REF) $(BCM_REVISION)

.PHONY: rpm
rpm: build
	scripts/build-rpm.sh $(GIT_REF) $(BCM_REVISION)
