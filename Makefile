# Variables
IMG ?= document-processing-platform:latest
CONTROLLER_IMG ?= document-processing-controller:latest

# Get the currently used golang install path
GOPATH ?= $(shell go env GOPATH)
GOBIN ?= $(GOPATH)/bin

# Tools
CONTROLLER_GEN ?= $(GOBIN)/controller-gen
KUSTOMIZE ?= $(GOBIN)/kustomize

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: test
test: ## Run tests
	go test ./... -coverprofile cover.out

.PHONY: test-unit
test-unit: ## Run unit tests only
	go test -v -short ./...

.PHONY: test-integration
test-integration: ## Run integration tests
	go test ./test/integration/... -tags=integration

.PHONY: test-auth
test-auth: ## Run authentication integration tests
	go test -v -run "TestAuth" ./tests/

.PHONY: test-performance
test-performance: ## Run performance/benchmark tests
	go test -v -bench=. -benchmem ./tests/

.PHONY: test-stress
test-stress: ## Run stress tests
	go test -v -run "TestStress" ./tests/

.PHONY: test-all
test-all: test-unit test-integration test-auth ## Run all tests

.PHONY: run-tests
run-tests: ## Run integration tests programmatically
	@echo "Running integration test suite..."
	go run ./tests/ run-tests

.PHONY: coverage
coverage: ## Generate test coverage report
	@echo "Generating coverage report..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary
	go build -o bin/server cmd/server/main.go
	go build -o bin/controller cmd/controller/main.go
	go build -o bin/processor cmd/processor/main.go

.PHONY: docker-build
docker-build: ## Build docker images
	docker build -t ${IMG} .
	docker build -f Dockerfile.controller -t ${CONTROLLER_IMG} .

.PHONY: docker-push
docker-push: ## Push docker images
	docker push ${IMG}
	docker push ${CONTROLLER_IMG}

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster
	cd config/manager && $(KUSTOMIZE) edit set image controller=${CONTROLLER_IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found -f -

##@ Tools

controller-gen: ## Download controller-gen locally if necessary
	test -s $(CONTROLLER_GEN) || GOBIN=$(GOBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

kustomize: ## Download kustomize locally if necessary
	test -s $(KUSTOMIZE) || GOBIN=$(GOBIN) go install sigs.k8s.io/kustomize/kustomize/v5@latest
