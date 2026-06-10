default: build

.PHONY: build test fmt vet lint install dev-install clean

BINARY   = terraform-provider-zcp
HOSTNAME = registry.terraform.io
NAMESPACE = zsoftly
NAME = zcp
OS_ARCH = $(shell go env GOOS)_$(shell go env GOARCH)

build:
	go build -o $(BINARY) ./

test:
	go test -v -count=1 ./...

test-race:
	go test -race -count=1 ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

install: build
	mkdir -p ~/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/0.1.0/$(OS_ARCH)
	mv $(BINARY) ~/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/0.1.0/$(OS_ARCH)/$(BINARY)

clean:
	rm -f $(BINARY)
