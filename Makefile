NS = techops-docker.maven.dev.tripadvisor.com/utils
REPO = go-kube-sidecar
VERSION ?= 0.1

PATH  := ~/go/bin:$(PATH)

all: fmt vet test build

run:
	@echo "==== go run ==="
	@go run main.go

fmt:
	@echo "==== go fmt ==="
	@go fmt ./...

vet:
	@echo "==== go vet ==="
	@go vet ./...

test:
	@echo "==== go test ==="
	@go test -cover ./...

build:
	@echo "==== docker build ==="
	docker build -t $(NS)/$(REPO):$(VERSION) .

push:
	@echo "==== docker build and push ==="
	docker build -t $(NS)/$(REPO):$(VERSION) .
	docker push $(NS)/$(REPO):$(VERSION)
