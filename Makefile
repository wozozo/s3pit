.PHONY: help build run test clean all docker-build docker-run docker-run-detached docker-stop docker-clean docker-push docker-shell

# Variables
BINARY_NAME=s3pit
GO=go
GOFLAGS=-v
DOCKER_IMAGE=s3pit
DOCKER_TAG=latest

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

# Docker commands
docker-build: ## Build Docker image
	@echo "${YELLOW}Building Docker image ${DOCKER_IMAGE}:${DOCKER_TAG}...${NC}"
	docker build -t ${DOCKER_IMAGE}:${DOCKER_TAG} .
	@echo "${GREEN}Docker image built successfully!${NC}"

docker-run: docker-build ## Build and run Docker container
	@echo "${YELLOW}Running Docker container...${NC}"
	docker run --rm -p 3333:3333 ${DOCKER_IMAGE}:${DOCKER_TAG}

docker-run-detached: docker-build ## Build and run Docker container in detached mode
	@echo "${YELLOW}Running Docker container in detached mode...${NC}"
	docker run -d --name ${DOCKER_IMAGE} -p 3333:3333 ${DOCKER_IMAGE}:${DOCKER_TAG}
	@echo "${GREEN}Container started with name: ${DOCKER_IMAGE}${NC}"

docker-stop: ## Stop running Docker container
	@echo "${YELLOW}Stopping Docker container...${NC}"
	docker stop ${DOCKER_IMAGE} || true
	docker rm ${DOCKER_IMAGE} || true
	@echo "${GREEN}Container stopped and removed!${NC}"

docker-clean: ## Remove Docker image and containers
	@echo "${YELLOW}Cleaning Docker artifacts...${NC}"
	docker stop ${DOCKER_IMAGE} 2>/dev/null || true
	docker rm ${DOCKER_IMAGE} 2>/dev/null || true
	docker rmi ${DOCKER_IMAGE}:${DOCKER_TAG} 2>/dev/null || true
	@echo "${GREEN}Docker cleanup complete!${NC}"

docker-push: docker-build ## Build and push Docker image to registry
	@echo "${YELLOW}Pushing Docker image to registry...${NC}"
	docker push ${DOCKER_IMAGE}:${DOCKER_TAG}
	@echo "${GREEN}Docker image pushed successfully!${NC}"

docker-shell: docker-build ## Run Docker container with shell access (for debugging)
	@echo "${YELLOW}Starting Docker container with shell...${NC}"
	docker run --rm -it --entrypoint /bin/sh ${DOCKER_IMAGE}:${DOCKER_TAG}

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