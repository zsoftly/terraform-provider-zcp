default: build

.PHONY: build test test-race fmt vet lint install install-terraform dev-install clean

BINARY    = terraform-provider-zcp
NAMESPACE = zsoftly
NAME      = zcp
VERSION   = 0.1.0
OS_ARCH   = $(shell go env GOOS)_$(shell go env GOARCH)

# OpenTofu (primary)
OTF_HOSTNAME   = registry.opentofu.org
OTF_PLUGIN_DIR = ~/.opentofu/plugins
OTF_ADDR       = $(OTF_HOSTNAME)/$(NAMESPACE)/$(NAME)

# Terraform (secondary)
TF_HOSTNAME   = registry.terraform.io
TF_PLUGIN_DIR = ~/.terraform.d/plugins
TF_ADDR       = $(TF_HOSTNAME)/$(NAMESPACE)/$(NAME)

# Build for OpenTofu (default).
build:
	go build -ldflags "-X main.providerAddress=$(OTF_ADDR)" -o $(BINARY) ./

test:
	go test -v -count=1 ./...

test-race:
	go test -race -count=1 ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

# Install into the local OpenTofu plugin cache (non-dev_overrides path).
install: build
	mkdir -p $(OTF_PLUGIN_DIR)/$(OTF_ADDR)/$(VERSION)/$(OS_ARCH)
	cp $(BINARY) $(OTF_PLUGIN_DIR)/$(OTF_ADDR)/$(VERSION)/$(OS_ARCH)/$(BINARY)

# Build with the Terraform registry address and install into the local Terraform plugin cache.
install-terraform:
	go build -ldflags "-X main.providerAddress=$(TF_ADDR)" -o $(BINARY) ./
	mkdir -p $(TF_PLUGIN_DIR)/$(TF_ADDR)/$(VERSION)/$(OS_ARCH)
	cp $(BINARY) $(TF_PLUGIN_DIR)/$(TF_ADDR)/$(VERSION)/$(OS_ARCH)/$(BINARY)

# Rebuild in-place for dev_overrides workflows.
dev-install: build
	@echo ""
	@echo "Binary built at $(CURDIR)/$(BINARY)"
	@echo "Ensure your CLI config points here via dev_overrides:"
	@echo "  ~/.tofurc      — \"$(OTF_ADDR)\" = \"$(CURDIR)\""
	@echo "  ~/.terraformrc — \"$(TF_ADDR)\"  = \"$(CURDIR)\""
	@echo ""

clean:
	rm -f $(BINARY)
