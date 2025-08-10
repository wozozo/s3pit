# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

S3pit is a lightweight S3-compatible storage server for local development and testing. The key differentiating feature is automatic bucket creation on first upload operations (PutObject, CopyObject, InitiateMultipartUpload), eliminating the need for explicit bucket creation in code.

## Build and Development Commands

### Core Commands
```bash
# Build the binary
make build
# or directly with Go
go build -o s3pit .

# Run tests
make test
# or directly with Go
go test -v ./...

# Format code
make fmt
# or directly with Go
go fmt ./...

# Run linter (installs golangci-lint if not present)
make lint

# Clean build artifacts and data
make clean
```

### Running the Server
```bash
# Build and run
make run

# Run with custom options
./s3pit serve --port 3333 --global-dir ./data

```

### Testing Scripts
```bash
./test_s3pit.sh     # Basic S3 operations test
./test_phase2.sh    # Advanced features test (multipart, copy, etc.)
```

## Architecture Overview

### Core Components

1. **Storage Interface** (`pkg/storage/storage.go`)
   - Defines the abstract Storage interface for all S3 operations
   - Two implementations: FileSystem (`filesystem.go`) and Memory (`memory.go`)
   - Handles bucket/object CRUD, multipart uploads, and metadata

2. **API Handler** (`pkg/api/handler.go`)
   - Implements S3-compatible REST API endpoints
   - Routes requests to appropriate storage backend
   - Handles XML request/response marshaling for S3 compatibility

3. **Authentication** (`pkg/auth/handler.go`)
   - Supports two auth modes: sigv4
   - AWS Signature V4 verification for production-like testing with presigned URL support

4. **Multi-tenancy** (`pkg/tenant/manager.go`)
   - Maps access keys to isolated storage directories
   - Configured via config.toml file

5. **Web Dashboard** (`pkg/dashboard/`)
   - Built-in web UI at `/dashboard`
   - Static assets in `static/`, templates in `templates/`
   - Real-time API logging and bucket/object management

6. **Server** (`pkg/server/server.go`)
   - Gin-based HTTP server setup
   - CORS configuration for browser access
   - Request routing and middleware setup

### Key Design Patterns

- **Auto-create buckets**: The system automatically creates buckets on write operations if they don't exist, configured via `config.AutoCreateBucket`
- **Metadata storage**: Object metadata stored as `.s3pit_meta.json` files alongside objects in filesystem storage
- **Multipart uploads**: Temporary parts stored in `.s3pit_multipart/` directory, assembled on completion
- **Streaming I/O**: Large files handled with io.Reader/Writer interfaces for memory efficiency

## Configuration

Environment variables are loaded in `internal/config/config.go`. Key variables:
- `S3PIT_PORT`: Server port (default: 3333)
- `S3PIT_GLOBAL_DIRECTORY`: Storage directory (default: ~/s3pit)
- `S3PIT_AUTH_MODE`: Authentication mode
- `S3PIT_IN_MEMORY`: Use memory storage instead of filesystem
- `S3PIT_AUTO_CREATE_BUCKET`: Enable auto-create (default: true)
- `S3PIT_CONFIG_FILE`: Path to config.toml for multi-tenancy

## Testing Approach

1. **Unit tests**: Located in `*_test.go` files, test individual components
2. **Integration tests**: Shell scripts (`test_*.sh`) test end-to-end S3 operations
3. **SDK tests**: `test_aws_sdk.js` and `test_boto3.py` verify SDK compatibility

Run specific test:
```bash
go test -v ./pkg/storage -run TestFileSystemStorage
```

## Important Notes

- The project uses Gin framework for HTTP handling and routing
- XML marshaling/unmarshaling for S3 API compatibility is critical
- File paths in filesystem storage use URL-safe encoding for special characters
- Dashboard is served from `/dashboard`, API endpoints from root path
- Project follows standard Go project layout with `cmd/`, `pkg/`, `internal/` directories