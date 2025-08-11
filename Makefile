.PHONY: help build run test clean all

# Variables
BINARY_NAME=s3pit
GO=go
GOFLAGS=-v

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m # No Color

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  ${GREEN}%-20s${NC} %s\n", $$1, $$2}'

build: ## Build the binary
	@echo "${YELLOW}Building ${BINARY_NAME}...${NC}"
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME) .
	@echo "${GREEN}Build complete!${NC}"

run: build ## Build and run the application
	@echo "${YELLOW}Starting ${BINARY_NAME}...${NC}"
	./$(BINARY_NAME)

serve: build ## Build and run the server with configuration validation
	@echo "${YELLOW}Starting ${BINARY_NAME} server with validation...${NC}"
	./$(BINARY_NAME) serve

test: ## Run tests
	@echo "${YELLOW}Running tests...${NC}"
	$(GO) test $(GOFLAGS) ./...
	@echo "${GREEN}Tests complete!${NC}"

clean: ## Clean build artifacts
	@echo "${YELLOW}Cleaning...${NC}"
	rm -f $(BINARY_NAME)
	rm -rf tmp/
	rm -rf data/
	@echo "${GREEN}Clean complete!${NC}"

# Installation commands
install-deps: ## Install Go dependencies
	@echo "${YELLOW}Installing dependencies...${NC}"
	$(GO) mod download
	$(GO) mod tidy
	@echo "${GREEN}Dependencies installed!${NC}"

# Linting and formatting
fmt: ## Format Go code
	@echo "${YELLOW}Formatting code...${NC}"
	$(GO) fmt ./...
	@echo "${GREEN}Formatting complete!${NC}"

lint: ## Run linter
	@echo "${YELLOW}Running linter...${NC}"
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@PATH="$$PATH:$$HOME/go/bin" golangci-lint run ./...
	@echo "${GREEN}Linting complete!${NC}"

all: build test lint fmt ## Run all checks
	@echo "${GREEN}All checks complete!${NC}"

# Release commands
release-patch: ## Create a patch release (v0.0.X)
	@echo "${YELLOW}Creating patch release...${NC}"
	git tag -a v$$(git describe --tags --abbrev=0 | awk -F. '{OFS="."; $$NF+=1; print}' | sed 's/v//') -m "Patch release"
	git push origin --tags
	@echo "${GREEN}Patch release created!${NC}"

release-minor: ## Create a minor release (v0.X.0)
	@echo "${YELLOW}Creating minor release...${NC}"
	git tag -a v$$(git describe --tags --abbrev=0 | awk -F. '{OFS="."; $$2+=1; $$3=0; print}' | sed 's/v//') -m "Minor release"
	git push origin --tags
	@echo "${GREEN}Minor release created!${NC}"

release-major: ## Create a major release (vX.0.0)
	@echo "${YELLOW}Creating major release...${NC}"
	git tag -a v$$(git describe --tags --abbrev=0 | awk -F. '{OFS="."; $$1+=1; $$2=0; $$3=0; print}' | sed 's/v//') -m "Major release"
	git push origin --tags
	@echo "${GREEN}Major release created!${NC}"