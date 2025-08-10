# S3pit

A lightweight S3-compatible storage server designed for **multi-project development** with **zero cognitive overhead**.

## 🚀 Why S3pit?

**The Problem**: Working on multiple applications simultaneously with traditional S3 solutions (MinIO, LocalStack) requires:
- Managing separate storage instances per project
- Remembering different ports and configurations
- Complex Docker setups that consume resources
- Isolated storage that's disconnected from your codebase

**The S3pit Solution**:
- ✅ **One server, multiple projects**: Single S3pit instance serves all your projects
- ✅ **Flexible storage options**: Repository-local (`~/project/data/`) OR centralized (`~/s3pit/data/`) - your choice
- ✅ **Descriptive access keys**: Use meaningful accessKeyIds like `"user-uploads-dev"` for easy project identification
- ✅ **Zero configuration switching**: Change projects without changing S3 settings
- ✅ **Automatic bucket creation**: No manual setup - just start uploading any bucket name
- ✅ **Minimal resource usage**: Lightweight single binary, no Docker overhead

## MinIO vs S3pit Comparison

| Feature | MinIO | S3pit |
|---------|-------|-------|
| **Multi-project setup** | Multiple instances/Docker containers | Single instance serves all projects |
| **Storage location** | Fixed centralized `/data` directory | Flexible: Repository-local OR centralized |
| **Bucket management** | Manual creation required | Automatic creation on first upload |
| **Configuration overhead** | Different ports/credentials per project | One config, automatic project isolation |
| **Resource usage** | Heavy (multiple instances) | Lightweight (single binary) |
| **Development workflow** | Switch contexts, remember configs | Seamless project switching |
| **Repository integration** | Storage separate from code | Optional: Storage lives with your code |

**Perfect for developers juggling multiple projects** - S3pit eliminates the mental overhead of managing separate S3 environments.

## Real-World Developer Scenario

### Step 1: One-Time Configuration

Create `~/.config/s3pit/tenants.json` (auto-created on first run, or customize it):

**Option A: Repository-Local Storage** (each project's data in its own repo)
```json
{
  "globalDir": "~/s3pit",
  "tenants": [
    {
      "accessKeyId": "ecommerce-dev",
      "secretAccessKey": "ecommerce-secret",
      "customDir": "~/src/github.com/yourname/ecommerce-app/data",
      "description": "E-commerce app development",
      "publicBuckets": ["product-images"]
    },
    {
      "accessKeyId": "blog-dev",
      "secretAccessKey": "blog-secret",
      "customDir": "~/src/github.com/yourname/blog-platform/data",
      "description": "Blog platform development",
      "publicBuckets": ["public-assets"]
    },
    {
      "accessKeyId": "images-dev",
      "secretAccessKey": "images-secret",
      "customDir": "~/src/github.com/yourname/image-processor/data",
      "description": "Image processor development",
      "publicBuckets": []
    }
  ]
}
```

**Option B: Centralized Storage** (all projects under one directory)
```json
{
  "globalDir": "~/s3pit",
  "tenants": [
    {
      "accessKeyId": "ecommerce-dev",
      "secretAccessKey": "ecommerce-secret",
      "description": "E-commerce app development"
    },
    {
      "accessKeyId": "blog-dev",
      "secretAccessKey": "blog-secret",
      "description": "Blog platform development"
    },
    {
      "accessKeyId": "images-dev",
      "secretAccessKey": "images-secret",
      "description": "Image processor development"
    }
  ]
}
```

> **💡 Auto-Organization**: When `customDir` is omitted, S3pit automatically organizes projects under the global `globalDir`:
> - `ecommerce-dev` → `~/s3pit/ecommerce-dev/`
> - `blog-dev` → `~/s3pit/blog-dev/`
> - `images-dev` → `~/s3pit/images-dev/`

**🎯 Pro Tip**: Use descriptive `accessKeyId` names for easy project identification! Each access key provides an isolated bucket namespace where you can create any bucket names you need. For example:
- `accessKeyId: "user-uploads-dev"` → Isolated storage for buckets like `avatars`, `documents`, `temp-files`
- `accessKeyId: "ecommerce-prod"` → Separate storage for buckets like `product-images`, `user-data`, `backups`

**Choose Your Style:**
- **Repository-Local**: Perfect for version control, project isolation, easy cleanup
- **Centralized**: Better for shared resources, cross-project data access, traditional workflow

### Step 2: Start S3pit (Once)

```bash
# Single command starts server for ALL projects
s3pit serve  # Runs on localhost:3333, serves all tenants
```

### Step 3: Develop Multiple Projects Simultaneously

**Option A: Repository-Local Storage** (when using specific `customDir` settings)
```bash
# Project 1: E-commerce app
cd ~/src/github.com/yourname/ecommerce-app
export AWS_ACCESS_KEY_ID=ecommerce-dev
export AWS_SECRET_ACCESS_KEY=ecommerce-secret
npm run dev  # Uploads go to ./data/ in THIS project

# Project 2: Blog platform (different terminal)
cd ~/src/github.com/yourname/blog-platform
export AWS_ACCESS_KEY_ID=blog-dev
export AWS_SECRET_ACCESS_KEY=blog-secret
npm run dev  # Uploads go to ./data/ in THIS project

# Result: Each project's S3 data lives in its own repository folder!
ls ~/src/github.com/yourname/ecommerce-app/data/     # E-commerce buckets
ls ~/src/github.com/yourname/blog-platform/data/    # Blog buckets
```

**Option B: Centralized Storage** (when using global `globalDir`)
```bash
# Project 1: E-commerce app
export AWS_ACCESS_KEY_ID=ecommerce-dev
export AWS_SECRET_ACCESS_KEY=ecommerce-secret
npm run dev  # Uploads go to ~/s3pit/ecommerce-dev/

# Project 2: Blog platform (different terminal)
export AWS_ACCESS_KEY_ID=blog-dev
export AWS_SECRET_ACCESS_KEY=blog-secret
npm run dev  # Uploads go to ~/s3pit/blog-dev/

# Result: All projects organized under one directory by accessKeyId
ls ~/s3pit/ecommerce-dev/     # E-commerce buckets
ls ~/s3pit/blog-dev/          # Blog buckets
```

**The Result**:
- 🎯 **Focus on coding**, not infrastructure management
- 🚀 **Instant project switching** without reconfiguration
- 📁 **Flexible organization** - choose repository-local OR centralized storage
- 🔄 **Team synchronization** - same setup works for everyone
- 💻 **Resource efficient** - one lightweight process serves everything
- 🏗️ **Automatic isolation** - each accessKeyId gets its own storage namespace

## Features

- **S3 Compatible API**: Implements core S3 operations with AWS SDK compatibility
- **Implicit Bucket Creation**: Automatically creates buckets on first upload (PutObject, CopyObject, InitiateMultipartUpload)
- **🚀 Repository-Local Storage**: Store S3 data directly in your project directories - reduces cognitive load and keeps everything organized
- **Web Dashboard**: Built-in web UI for managing buckets and objects
- **Multiple Storage Backends**: File system or in-memory storage
- **Authentication Modes**: AWS Signature V4
- **Multi-tenancy Support**: Map different access keys to separate directories
- **Path-Style URLs**: Enforces path-style access for compatibility
- **Streaming I/O**: Efficient handling of large files with streaming
- **Multipart Upload**: Full support for S3 multipart upload operations
- **Performance Optimized**: Buffered I/O, metadata caching, per-bucket locking, and memory pooling
- **Enhanced Logging**: Structured logging with levels, filtering, rotation, and real-time dashboard viewer
- **Comprehensive Error Handling**: S3-compatible XML error responses

## Installation

### Install via Go

```bash
go install github.com/wozozo/s3pit@latest
s3pit serve
```

### Build from Source

```bash
# Requirements: Go 1.24+
git clone https://github.com/wozozo/s3pit.git
cd s3pit

# Build using Make
make build

# Or using Go directly
go build -o s3pit .

# Run the server
./s3pit serve
```

### Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/wozozo/s3pit/releases):

```bash
# Linux (amd64)
wget https://github.com/wozozo/s3pit/releases/latest/download/s3pit-linux-amd64
chmod +x s3pit-linux-amd64
./s3pit-linux-amd64 serve

# macOS (arm64)
wget https://github.com/wozozo/s3pit/releases/latest/download/s3pit-darwin-arm64
chmod +x s3pit-darwin-arm64
./s3pit-darwin-arm64 serve

# Windows
# Download s3pit-windows-amd64.exe from releases page
s3pit-windows-amd64.exe serve
```

## Quick Start Guide

### 1. Start S3pit Server

```bash
# Simple setup
s3pit serve

# With AWS Signature V4 authentication
s3pit serve --auth-mode sigv4

# In-memory storage for testing (data lost on restart)
s3pit serve --in-memory

# Custom data directory with logging
s3pit serve --global-dir /var/s3pit/data --log-level debug
```

### 2. Configure Your Application

#### AWS SDK for JavaScript/Node.js
```javascript
import { S3Client } from "@aws-sdk/client-s3";

const s3 = new S3Client({
  endpoint: "http://localhost:3333",
  region: "us-east-1",  // Any region works
  credentials: {
    accessKeyId: "local-dev",
    secretAccessKey: "local-dev-secret"
  },
  forcePathStyle: true  // Required for S3pit
});
```


#### AWS CLI
```bash
# Configure AWS CLI
aws configure set aws_access_key_id local-dev
aws configure set aws_secret_access_key local-dev-secret
aws configure set region us-east-1

# Use with endpoint URL
export AWS_ENDPOINT_URL=http://localhost:3333
aws s3 ls

# Or specify per command
aws s3 ls --endpoint-url http://localhost:3333
```

### 3. Test Your Setup

```bash
# Create a test file
echo "Hello S3pit!" > test.txt

# Upload (bucket auto-created if it doesn't exist)
aws s3 cp test.txt s3://test-bucket/ --endpoint-url http://localhost:3333

# List buckets
aws s3 ls --endpoint-url http://localhost:3333

# List objects
aws s3 ls s3://test-bucket/ --endpoint-url http://localhost:3333

# Download
aws s3 cp s3://test-bucket/test.txt downloaded.txt --endpoint-url http://localhost:3333

# Verify
cat downloaded.txt  # Should print: Hello S3pit!
```


### Use with AWS SDK (Node.js)

```javascript
import { S3Client, PutObjectCommand, CopyObjectCommand } from "@aws-sdk/client-s3";

const client = new S3Client({
  endpoint: "http://localhost:3333",
  region: "us-east-1",
  credentials: {
    accessKeyId: "local-dev",
    secretAccessKey: "local-dev-secret"
  },
  forcePathStyle: true
});

// Upload object (bucket auto-created if not exists)
const putCommand = new PutObjectCommand({
  Bucket: "my-bucket",
  Key: "test.txt",
  Body: "Hello, S3pit!"
});
await client.send(putCommand);

// Copy object
const copyCommand = new CopyObjectCommand({
  Bucket: "my-bucket",
  Key: "test-copy.txt",
  CopySource: "/my-bucket/test.txt"
});
await client.send(copyCommand);
```

## Web Dashboard

S3pit includes a built-in web dashboard for easy management and monitoring.

### Features
- **Bucket Management**: Create, list, and delete buckets
- **Object Browser**: Upload, download, delete, and browse objects
- **Presigned URL Generator**: Generate presigned URLs for GET/PUT operations
- **Tenant Viewer**: View multi-tenant mappings
- **Enhanced API Logs**:
  - Real-time request/response logging with detailed information
  - Advanced filtering by log level, operation type, time range, and text search
  - Export logs as JSON for external analysis
  - Auto-refresh for live monitoring
  - Color-coded entries based on severity

### Access
Navigate to `http://localhost:3333/dashboard` when the server is running.

## Configuration

### Command Line Options

```bash
s3pit serve [options]

Options:
  --host string               Server host (default "0.0.0.0")
  --port int                  Server port (default 3333)
  --global-dir string         Override global directory path
  --auth-mode string          Authentication mode: sigv4 (default "sigv4")
  --tenants-file string       Path to tenants.json for multi-tenancy
  --in-memory                 Use in-memory storage
  --dashboard                 Enable web dashboard (default true)
  --auto-create-bucket        Auto-create buckets on upload (default true)
  --log-level string          Log level: debug|info|warn|error (default "info")
  --log-dir string            Directory for log files (default "./logs")
  --no-dashboard              Disable web dashboard
  --max-object-size int       Maximum object size in bytes (default 5368709120)
```

### Environment Variables

All command-line options can be configured via environment variables with the `S3PIT_` prefix:

| Environment Variable | Type | Default | Description |
|---------------------|------|---------|-------------|
| `S3PIT_HOST` | string | "0.0.0.0" | Server bind address. Use "127.0.0.1" for localhost only |
| `S3PIT_PORT` | int | 3333 | Server port. Common alternatives: 9001, 8080 |
| `S3PIT_GLOBAL_DIRECTORY` | string | "~/s3pit" | Global directory for storing buckets and objects |
| `S3PIT_AUTH_MODE` | string | | Authentication mode:<br>• `sigv4`: Full AWS Signature V4 validation |
| `S3PIT_IN_MEMORY` | bool | false | Store all data in memory (lost on restart) |
| `S3PIT_AUTO_CREATE_BUCKET` | bool | true | Auto-create buckets on first upload |
| `S3PIT_LOG_LEVEL` | string | "info" | Minimum log level: debug, info, warn, error |
| `S3PIT_LOG_DIR` | string | "./logs" | Directory for log files |
| `S3PIT_ENABLE_FILE_LOG` | bool | true | Write logs to files |
| `S3PIT_ENABLE_CONSOLE_LOG` | bool | true | Write logs to console |
| `S3PIT_LOG_ROTATION_SIZE` | int | 104857600 | Log rotation size in bytes (default 100MB) |
| `S3PIT_MAX_LOG_ENTRIES` | int | 10000 | Max in-memory log entries for dashboard |
| `S3PIT_MAX_OBJECT_SIZE` | int | 5368709120 | Max object size in bytes (default 5GB) |
| `S3PIT_ENABLE_DASHBOARD` | bool | true | Enable web dashboard at /dashboard |
| `S3PIT_TENANTS_FILE` | string | "~/.config/s3pit/tenants.json" | Path to tenants.json for multi-tenancy (auto-created) |

### Configuration Examples

#### Development Setup
```bash
# Minimal setup for local development
export S3PIT_AUTO_CREATE_BUCKET=true
export S3PIT_LOG_LEVEL=debug
s3pit serve
```

#### Production Setup
```bash
# Secure setup with persistent storage
export S3PIT_GLOBAL_DIRECTORY=/var/lib/s3pit/data
export S3PIT_LOG_LEVEL=info
s3pit serve
```

### Logging

S3pit provides comprehensive logging capabilities for monitoring and debugging:

#### Features
- **Structured JSON logging**: Each log entry contains detailed metadata including request/response bodies, headers, and S3 operation types
- **Multiple log levels**: DEBUG, INFO, WARN, ERROR with configurable minimum level
- **Automatic log rotation**: Rotates log files when they exceed size limits (default 100MB)
- **Dual output**: Simultaneous console (with color coding) and file logging
- **Operation tracking**: Automatically identifies S3 operation types (PutObject, GetObject, etc.)
- **Performance metrics**: Tracks request duration for all API calls
- **Sensitive data filtering**: Automatically removes Authorization headers from logs

#### Log Files
Logs are stored in JSON format at `./logs/s3pit_YYYY-MM-DD.log`. Example entry:
```json
{
  "id": "1754736444664422000-93654",
  "timestamp": "2025-08-09T19:47:24.663904+09:00",
  "level": "INFO",
  "method": "PUT",
  "path": "/test-bucket/test.txt",
  "statusCode": 200,
  "duration": 515459,
  "clientIP": "::1",
  "bucket": "test-bucket",
  "key": "/test.txt",
  "operation": "PutObject"
}
```

#### Dashboard Integration
The web dashboard provides a powerful log viewer with:
- Real-time log streaming with auto-refresh
- Filtering by level, operation, time range, and text search
- Export functionality for external analysis
- Color-coded entries for quick status identification

### Multi-tenancy

S3pit supports multi-tenancy by mapping different access keys to isolated storage directories.

#### Automatic Configuration Setup

On first run, S3pit automatically:
1. Creates `~/.config/s3pit/` directory
2. Generates a default `tenants.json` with sample credentials
3. Loads `~/.config/s3pit/tenants.json` by default (if no `--tenants-file` specified)

Default `tenants.json` created at `~/.config/s3pit/tenants.json`:
```json
{
  "globalDir": "~/s3pit/data",
  "tenants": [
    {
      "accessKeyId": "local-dev",
      "secretAccessKey": "local-dev-secret",
      "description": "Local development with public assets (public-*, static-*, cdn-*)",
      "publicBuckets": ["public-*", "static-*", "cdn-*"]
    },
    {
      "accessKeyId": "test-app",
      "secretAccessKey": "test-app-secret",
      "description": "Test application with specific public buckets",
      "publicBuckets": ["assets", "downloads"]
    },
    {
      "accessKeyId": "private-app",
      "secretAccessKey": "private-app-secret",
      "description": "Private application (all buckets require authentication)",
      "publicBuckets": []
    }
  ]
}
```

**Default Tenants Explained:**
- **local-dev**: Perfect for frontend development with public asset serving. Buckets matching `public-*`, `static-*`, or `cdn-*` are automatically public (read-only)
- **test-app**: For testing mixed scenarios with specific public buckets (`assets`, `downloads`)
- **private-app**: For applications requiring authentication for all operations

**Configuration Properties:**
- `globalDir` (string, required): Global data directory for all tenants. Must be absolute path (starting with `/`) or home directory path (starting with `~/`)
- `accessKeyId` (string, required): Access key identifier for authentication
- `secretAccessKey` (string, required): Secret access key for authentication
- `customDir` (string, optional): Tenant-specific storage directory path. If omitted, uses `{globalDir}/{accessKeyId}/`. Must be absolute path (starting with `/`) or home directory path (starting with `~/`)
- `description` (string, optional): Human-readable description of the tenant
- `publicBuckets` (array, optional): List of bucket names that allow public access without authentication

#### Custom Tenant Configuration

**🚀 Key Advantage: Repository-Local Storage**

S3pit's unique selling point is **flexible directory mapping** that reduces cognitive load during development. Instead of managing separate storage locations, you can store S3 uploads directly within your project repositories:

```json
{
  "globalDir": "~/s3pit",
  "tenants": [
    {
      "accessKeyId": "app1-dev",
      "secretAccessKey": "app1-secret",
      "customDir": "~/src/github.com/example-user/app1/data",
      "description": "App1 development storage",
      "publicBuckets": []
    },
    {
      "accessKeyId": "app2-dev",
      "secretAccessKey": "app2-secret",
      "customDir": "~/src/github.com/example-user/app2/data",
      "description": "App2 development storage",
      "publicBuckets": ["public-assets"]
    }
  ]
}
```

**Benefits of Repository-Local Storage:**
- ✅ **Reduced Cognitive Load**: No need to remember separate storage locations
- ✅ **Version Control Ready**: Upload data lives alongside your code
- ✅ **Project Isolation**: Each project gets its own S3 namespace
- ✅ **Easy Cleanup**: Delete the project directory to remove everything
- ✅ **Seamless Development**: Switch between projects without configuration changes
- ✅ **Team Collaboration**: Same relative paths work for all developers

**Real-World Development Workflow:**
```bash
# Developer working on multiple projects
cd ~/src/github.com/example-user/app1
npm run dev  # App uses app1-dev credentials, stores in ./data/

cd ~/src/github.com/example-user/app2
npm run dev  # App uses app2-dev credentials, stores in ./data/

# No mental overhead switching between projects!
# Each project's uploaded files are right there in the repository
ls app1/data/  # Shows buckets and objects for app1
ls app2/data/  # Shows buckets and objects for app2
```

**Project Structure Example:**
```
~/src/github.com/example-user/
├── app1/
│   ├── src/
│   ├── package.json
│   └── data/              ← S3pit storage for app1
│       ├── user-uploads/
│       ├── temp-files/
│       └── .s3pit_meta.json
└── app2/
    ├── src/
    ├── package.json
    └── data/              ← S3pit storage for app2
        ├── public-assets/
        └── .s3pit_meta.json
```

**Advanced Configuration Examples:**

```json
{
  "globalDir": "/var/lib/s3pit/data",
  "tenants": [
    {
      "accessKeyId": "development",
      "secretAccessKey": "dev-secret-key",
      "customDir": "/var/lib/s3pit/dev",
      "description": "Development environment",
      "publicBuckets": ["test-uploads", "temp-files"]
    },
    {
      "accessKeyId": "production",
      "secretAccessKey": "prod-secret-key",
      "description": "Production environment - uses global globalDir",
      "publicBuckets": []
    }
  ]
}
```

**Storage Directory Resolution:**

S3pit uses a simple priority system to determine where to store data:

1. **Tenant-specific `customDir`** (if specified): `{tenant.customDir}/{bucket}/{object}`
2. **Global `globalDir` + accessKeyId**: `{globalDir}/{accessKeyId}/{bucket}/{object}`

**Examples:**
```bash
# Configuration:
{
  "globalDir": "~/s3pit",
  "tenants": [
    {"accessKeyId": "project-a", ...},                    # Uses global globalDir
    {"accessKeyId": "project-b", "customDir": "~/myapp/data", ...}  # Uses specific directory
  ]
}

# Storage paths:
# project-a uploads → ~/s3pit/project-a/my-bucket/file.txt
# project-b uploads → ~/myapp/data/my-bucket/file.txt
```

Run with custom tenants file:
```bash
# Use custom tenants file
./s3pit serve --tenants-file /path/to/tenants.json --auth-mode sigv4

# Use default ~/.config/s3pit/tenants.json
./s3pit serve --auth-mode sigv4
```

## Public Buckets

S3pit supports public bucket access, allowing certain buckets to be accessed without authentication for read operations. This is useful for serving static assets, public downloads, or development scenarios where read-only public access is needed.

### Configuration

Configure public buckets in your `tenants.json`:

```json
{
  "globalDir": "~/s3pit",
  "tenants": [
    {
      "accessKeyId": "app-dev",
      "secretAccessKey": "app-secret",
      "customDir": "~/src/app/data",
      "description": "Application with public assets",
      "publicBuckets": ["static-assets", "downloads", "public-*"]
    }
  ]
}
```

### Access Control

Public buckets have the following access control behavior:

| Operation | Without Authentication | With Authentication (Header/Presigned URL) |
|-----------|------------------------|-------------------------------------------|
| **GET** | ✅ Allowed | ✅ Allowed |
| **HEAD** | ✅ Allowed | ✅ Allowed |
| **OPTIONS** | ✅ Allowed | ✅ Allowed |
| **PUT** | ❌ Denied | ✅ Allowed |
| **POST** | ❌ Denied | ✅ Allowed |
| **DELETE** | ❌ Denied | ✅ Allowed |

### Features

- **Public Read Access**: GET/HEAD/OPTIONS operations allowed without authentication
- **Authenticated Write Access**: Write operations (PUT/POST/DELETE) require valid authentication
- **Presigned URL Support**: Enables secure temporary upload/delete permissions via presigned URLs
- **Wildcard Support**: Use patterns like `"public-*"` to match multiple buckets
- **Access Logging**: Clearly identifies public vs authenticated access in logs

### Usage Examples

```bash
# Public bucket - no authentication needed for reading
curl http://localhost:3333/static-assets/logo.png  # ✅ Works

# Direct write without authentication - denied
curl -X PUT http://localhost:3333/static-assets/new-file.txt -d "data"
# ❌ Error: Public buckets require authentication for write operations

# Write with authentication - allowed
export AWS_ACCESS_KEY_ID=app-dev
export AWS_SECRET_ACCESS_KEY=app-secret
aws s3 cp file.txt s3://static-assets/ --endpoint-url http://localhost:3333
# ✅ Upload succeeds with authentication

# Generate presigned URL for temporary upload permission
aws s3 presign s3://static-assets/upload.txt \
  --endpoint-url http://localhost:3333 \
  --expires-in 3600

# Upload using presigned URL (no additional auth needed)
curl -X PUT -T file.txt "$PRESIGNED_URL"
# ✅ Upload succeeds with presigned URL

# Private bucket - requires authentication for all operations
curl http://localhost:3333/private-data/file.txt
# ❌ Error: Access Denied
```

### Common Use Case: Frontend Applications

This design is perfect for frontend applications that need:
1. **Public read access** for serving assets (images, CSS, JS)
2. **Secure uploads** via presigned URLs from the backend

```javascript
// Frontend: Display public image (no auth needed)
<img src="http://localhost:3333/static-assets/logo.png" />

// Backend: Generate presigned URL for upload
const { getSignedUrl } = require("@aws-sdk/s3-request-presigner");
const { PutObjectCommand } = require("@aws-sdk/client-s3");

const command = new PutObjectCommand({
  Bucket: "static-assets",
  Key: "user-upload.jpg"
});

const presignedUrl = await getSignedUrl(s3Client, command, { 
  expiresIn: 3600 
});

// Frontend: Upload using presigned URL
await fetch(presignedUrl, {
  method: 'PUT',
  body: fileData
});
```

### Security Notes

- Public buckets allow unauthenticated read access only
- All write operations require proper authentication (AWS Signature V4 or presigned URL)
- Each tenant can define their own public buckets
- Public access is logged with `Type: public` for audit purposes
- Presigned URLs respect the authentication requirement for write operations

## API Compatibility Matrix

### S3 API Operations Support

| Category | Operation | Status | Notes |
|----------|-----------|--------|-------|
| **Bucket Operations** | | | |
| | CreateBucket | ✅ Full | Idempotent, auto-create on upload |
| | DeleteBucket | ✅ Full | Only empty buckets |
| | ListBuckets | ✅ Full | Returns all buckets |
| | HeadBucket | ✅ Full | Check bucket existence |
| | GetBucketLocation | ❌ Not Implemented | Returns fixed region |
| | GetBucketVersioning | ❌ Not Implemented | No versioning support |
| **Object Operations** | | | |
| | PutObject | ✅ Full | Auto bucket creation, streaming |
| | GetObject | ✅ Full | Range requests, streaming |
| | DeleteObject | ✅ Full | Idempotent |
| | DeleteObjects | ✅ Full | Batch delete with XML |
| | HeadObject | ✅ Full | Returns metadata |
| | CopyObject | ✅ Full | Server-side copy |
| | ListObjects | ⚠️ Partial | V1 API limited support |
| | ListObjectsV2 | ✅ Full | Prefix, delimiter, pagination |
| **Multipart Upload** | | | |
| | InitiateMultipartUpload | ✅ Full | Auto bucket creation |
| | UploadPart | ✅ Full | Part size validation |
| | CompleteMultipartUpload | ✅ Full | XML part list |
| | AbortMultipartUpload | ✅ Full | Cleanup temp files |
| | ListParts | ❌ Not Implemented | |
| | ListMultipartUploads | ❌ Not Implemented | |
| **Access Control** | | | |
| | PutBucketAcl | ❌ Not Implemented | |
| | GetBucketAcl | ❌ Not Implemented | |
| | PutObjectAcl | ❌ Not Implemented | |
| | GetObjectAcl | ❌ Not Implemented | |
| **Advanced Features** | | | |
| | GetObjectTagging | ❌ Not Implemented | |
| | PutObjectTagging | ❌ Not Implemented | |
| | DeleteObjectTagging | ❌ Not Implemented | |
| | GetBucketLifecycle | ❌ Not Implemented | |
| | PutBucketLifecycle | ❌ Not Implemented | |
| | GetBucketNotification | ❌ Not Implemented | |
| | PutBucketNotification | ❌ Not Implemented | |
| | SelectObjectContent | ❌ Not Implemented | S3 Select queries |
| | GetObjectLockConfiguration | ❌ Not Implemented | |
| | PutObjectLockConfiguration | ❌ Not Implemented | |



## Common Use Cases

### 1. Local Development Environment

Replace AWS S3 in your local development setup:

```javascript
// development.config.js
const config = {
  s3: {
    endpoint: process.env.S3_ENDPOINT || 'http://localhost:3333',
    credentials: {
      accessKeyId: 's3pitadmin',
      secretAccessKey: 's3pitadmin'
    },
    forcePathStyle: true
  }
};

const s3Client = new S3Client(config.s3);
```

### 2. CI/CD Pipeline Testing

```yaml
# .github/workflows/test.yml
services:
  s3pit:
    image: ghcr.io/wozozo/s3pit:latest
    ports:
      - 3333:3333
    env:
      S3PIT_IN_MEMORY: true

steps:
  - name: Run tests
    env:
      AWS_ENDPOINT_URL: http://localhost:3333
    run: npm test
```

### 3. Multi-Tenant Development

Set up isolated storage for different clients:

```bash
# Create tenants.json with isolated directories
s3pit serve --tenants-file tenants.json

# Each access key gets its own storage directory
AWS_ACCESS_KEY_ID=customer1 AWS_SECRET_ACCESS_KEY=customer1secret \
  aws s3 ls --endpoint-url http://localhost:3333
```

## Performance Features

S3pit is designed for efficient local development and testing:

### Built-in Features
The filesystem storage backend includes:
- **Per-Bucket Locking**: Reduces lock contention with bucket-level locks instead of global locks
- **Atomic File Operations**: Uses temporary files and atomic renames for data consistency
- **Streaming I/O**: Direct file streaming for efficient memory usage

### Performance Tuning
```bash
# For maximum performance with small files
export S3PIT_IN_MEMORY=true  # Keep all data in memory

# For large file workloads
export S3PIT_MAX_OBJECT_SIZE=10737418240  # 10GB
```

## Debug Mode

Enable debug logging for detailed troubleshooting:

```bash
# Via environment variable
export S3PIT_LOG_LEVEL=debug
s3pit serve

# Via command line
s3pit serve --log-level debug

# Check logs
tail -f ./logs/s3pit_*.log | jq '.'
```

## Development

### Requirements
- Go 1.24 or higher

## License

MIT License - see [LICENSE](LICENSE) file for details