const { createApp } = Vue;

createApp({
    data() {
        return {
            activeTab: 'buckets',
            buckets: [],
            objects: [],
            commonPrefixes: [],
            tenants: [],
            logs: [],
            selectedBucket: '',
            newBucketName: '',
            uploadKey: '',
            prefix: '',
            presignedBucket: '',
            presignedKey: '',
            presignedOperation: 'GET',
            presignedExpires: 3600,
            presignedContentType: '',
            generatedURL: '',
            authConfig: {
                authMode: 'sigv4',
                region: 'us-east-1',
                accessKeyId: ''
            },
            credentials: {
                accessKeyId: '',
                secretAccessKey: ''
            },
            autoRefreshLogs: false,
            autoRefreshInterval: null,
            logFilters: {
                level: '',
                operation: '',
                startTime: '',
                endTime: '',
                searchText: ''
            },
            logLimit: 100,
            availableOperations: [
                'ListBuckets', 'CreateBucket', 'DeleteBucket', 'HeadBucket',
                'ListObjects', 'GetObject', 'PutObject', 'DeleteObject', 'HeadObject',
                'CopyObject', 'InitiateMultipartUpload', 'UploadPart', 
                'CompleteMultipartUpload', 'AbortMultipartUpload'
            ],
            toast: {
                show: false,
                message: '',
                type: 'info'
            }
        };
    },
    mounted() {
        this.loadBuckets();
        this.loadAuthConfig();
        this.loadTenants();
        this.loadLogs();
    },
    computed: {
        canGeneratePresignedURL() {
            // Either use server defaults or both credentials must be provided
            return !this.credentials.accessKeyId || 
                   (this.credentials.accessKeyId && this.credentials.secretAccessKey);
        }
    },
    methods: {
        async loadBuckets() {
            try {
                const response = await fetch('/dashboard/api/buckets');
                const data = await response.json();
                this.buckets = data.buckets || [];
                this.showToast('Buckets loaded', 'success');
            } catch (error) {
                this.showToast('Failed to load buckets: ' + error.message, 'error');
            }
        },

        async createBucket() {
            if (!this.newBucketName) {
                this.showToast('Please enter a bucket name', 'error');
                return;
            }

            try {
                const response = await fetch('/dashboard/api/buckets', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ name: this.newBucketName })
                });

                if (response.ok) {
                    this.showToast('Bucket created successfully', 'success');
                    this.newBucketName = '';
                    await this.loadBuckets();
                } else {
                    const error = await response.json();
                    this.showToast('Failed to create bucket: ' + error.error, 'error');
                }
            } catch (error) {
                this.showToast('Failed to create bucket: ' + error.message, 'error');
            }
        },

        async deleteBucket(bucketName) {
            if (!confirm(`Are you sure you want to delete bucket "${bucketName}"?`)) {
                return;
            }

            try {
                const response = await fetch(`/dashboard/api/buckets/${bucketName}`, {
                    method: 'DELETE'
                });

                if (response.ok) {
                    this.showToast('Bucket deleted successfully', 'success');
                    await this.loadBuckets();
                    if (this.selectedBucket === bucketName) {
                        this.selectedBucket = '';
                        this.objects = [];
                    }
                } else {
                    const error = await response.json();
                    this.showToast('Failed to delete bucket: ' + error.error, 'error');
                }
            } catch (error) {
                this.showToast('Failed to delete bucket: ' + error.message, 'error');
            }
        },

        selectBucket(bucketName) {
            this.selectedBucket = bucketName;
            this.activeTab = 'objects';
            this.prefix = '';
            this.loadObjects();
        },

        async loadObjects() {
            if (!this.selectedBucket) return;

            try {
                const params = new URLSearchParams();
                if (this.prefix) params.append('prefix', this.prefix);
                params.append('delimiter', '/');

                const response = await fetch(`/dashboard/api/buckets/${this.selectedBucket}/objects?${params}`);
                const data = await response.json();
                this.objects = data.objects || [];
                this.commonPrefixes = data.commonPrefixes || [];
                this.showToast('Objects loaded', 'success');
            } catch (error) {
                this.showToast('Failed to load objects: ' + error.message, 'error');
            }
        },

        navigateToPrefix(prefix) {
            this.prefix = prefix;
            this.loadObjects();
        },

        handleFileSelect(event) {
            const file = event.target.files[0];
            if (file && !this.uploadKey) {
                this.uploadKey = file.name;
            }
        },

        async uploadObject() {
            const fileInput = this.$refs.fileInput;
            if (!fileInput.files.length) {
                this.showToast('Please select a file to upload', 'error');
                return;
            }

            const file = fileInput.files[0];
            const formData = new FormData();
            formData.append('file', file);
            if (this.uploadKey) {
                formData.append('key', this.prefix + this.uploadKey);
            }

            try {
                const response = await fetch(`/dashboard/api/buckets/${this.selectedBucket}/objects`, {
                    method: 'POST',
                    body: formData
                });

                if (response.ok) {
                    this.showToast('File uploaded successfully', 'success');
                    this.uploadKey = '';
                    fileInput.value = '';
                    await this.loadObjects();
                } else {
                    const error = await response.json();
                    this.showToast('Failed to upload file: ' + error.error, 'error');
                }
            } catch (error) {
                this.showToast('Failed to upload file: ' + error.message, 'error');
            }
        },

        async deleteObject(key) {
            if (!confirm(`Are you sure you want to delete "${key}"?`)) {
                return;
            }

            try {
                const response = await fetch(`/dashboard/api/buckets/${this.selectedBucket}/objects/${encodeURIComponent(key)}`, {
                    method: 'DELETE'
                });

                if (response.ok) {
                    this.showToast('Object deleted successfully', 'success');
                    await this.loadObjects();
                } else {
                    const error = await response.json();
                    this.showToast('Failed to delete object: ' + error.error, 'error');
                }
            } catch (error) {
                this.showToast('Failed to delete object: ' + error.message, 'error');
            }
        },

        downloadObject(key) {
            window.open(`/${this.selectedBucket}/${key}`, '_blank');
        },

        async loadAuthConfig() {
            try {
                const response = await fetch('/dashboard/api/auth-config');
                if (response.ok) {
                    this.authConfig = await response.json();
                }
            } catch (error) {
                console.error('Failed to load auth config:', error);
            }
        },

        async generatePresignedURL() {
            if (!this.presignedBucket || !this.presignedKey) {
                this.showToast('Please select a bucket and enter an object key', 'error');
                return;
            }

            if (this.credentials.accessKeyId && !this.credentials.secretAccessKey) {
                this.showToast('Secret access key is required when providing access key ID', 'error');
                return;
            }

            try {
                const requestBody = {
                    bucket: this.presignedBucket,
                    key: this.presignedKey,
                    operation: this.presignedOperation,
                    expires: this.presignedExpires
                };
                
                // Add content type for PUT requests
                if (this.presignedOperation === 'PUT' && this.presignedContentType) {
                    requestBody.contentType = this.presignedContentType;
                }
                
                // Add credentials if provided
                if (this.credentials.accessKeyId && this.credentials.secretAccessKey) {
                    requestBody.accessKeyId = this.credentials.accessKeyId;
                    requestBody.secretAccessKey = this.credentials.secretAccessKey;
                }
                
                const response = await fetch('/dashboard/api/presigned-url', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify(requestBody)
                });

                if (response.ok) {
                    const data = await response.json();
                    this.generatedURL = data.url;
                    this.showToast('Presigned URL generated', 'success');
                } else {
                    const error = await response.json();
                    this.showToast('Failed to generate URL: ' + error.error, 'error');
                }
            } catch (error) {
                this.showToast('Failed to generate URL: ' + error.message, 'error');
            }
        },

        async loadTenants() {
            try {
                const response = await fetch('/dashboard/api/tenants');
                const data = await response.json();
                this.tenants = data.tenants || [];
                this.showToast('Tenants loaded', 'success');
            } catch (error) {
                this.showToast('Failed to load tenants: ' + error.message, 'error');
            }
        },

        copyToClipboard(text) {
            navigator.clipboard.writeText(text).then(() => {
                this.showToast('Copied to clipboard', 'success');
            }).catch(() => {
                this.showToast('Failed to copy to clipboard', 'error');
            });
        },

        formatDate(dateStr) {
            if (!dateStr) return '';
            const date = new Date(dateStr);
            return date.toLocaleString();
        },

        formatSize(bytes) {
            if (bytes === 0) return '0 Bytes';
            const k = 1024;
            const sizes = ['Bytes', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        },

        formatDuration(nanoseconds) {
            if (!nanoseconds) return '0ms';
            const ms = nanoseconds / 1000000;
            if (ms < 1000) {
                return ms.toFixed(2) + 'ms';
            }
            return (ms / 1000).toFixed(2) + 's';
        },

        async loadLogs() {
            try {
                let url = `/dashboard/api/logs?limit=${this.logLimit}`;
                
                // Add filters to URL
                if (this.logFilters.level) {
                    url += `&level=${this.logFilters.level}`;
                }
                if (this.logFilters.operation) {
                    url += `&operation=${this.logFilters.operation}`;
                }
                if (this.logFilters.startTime) {
                    url += `&start_time=${new Date(this.logFilters.startTime).toISOString()}`;
                }
                if (this.logFilters.endTime) {
                    url += `&end_time=${new Date(this.logFilters.endTime).toISOString()}`;
                }
                
                const response = await fetch(url);
                const data = await response.json();
                this.logs = data.logs || [];
                
                // Filter by search text on client side
                if (this.logFilters.searchText) {
                    const searchLower = this.logFilters.searchText.toLowerCase();
                    this.logs = this.logs.filter(log => {
                        return (log.path && log.path.toLowerCase().includes(searchLower)) ||
                               (log.bucket && log.bucket.toLowerCase().includes(searchLower)) ||
                               (log.key && log.key.toLowerCase().includes(searchLower)) ||
                               (log.message && log.message.toLowerCase().includes(searchLower)) ||
                               (log.error && log.error.toLowerCase().includes(searchLower));
                    });
                }
                
                this.showToast('Logs loaded', 'success');
            } catch (error) {
                this.showToast('Failed to load logs: ' + error.message, 'error');
            }
        },
        
        clearLogFilters() {
            this.logFilters = {
                level: '',
                operation: '',
                startTime: '',
                endTime: '',
                searchText: ''
            };
            this.loadLogs();
        },
        
        exportLogs() {
            const logsJson = JSON.stringify(this.logs, null, 2);
            const blob = new Blob([logsJson], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `s3pit-logs-${new Date().toISOString()}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            this.showToast('Logs exported', 'success');
        },

        toggleAutoRefresh() {
            if (this.autoRefreshLogs) {
                this.autoRefreshInterval = setInterval(() => {
                    this.loadLogs();
                }, 2000); // Refresh every 2 seconds
            } else {
                if (this.autoRefreshInterval) {
                    clearInterval(this.autoRefreshInterval);
                    this.autoRefreshInterval = null;
                }
            }
        },

        getLogClass(log) {
            if (log.level === 'ERROR') return 'log-error';
            if (log.level === 'WARN') return 'log-warning';
            if (log.statusCode) {
                if (log.statusCode >= 200 && log.statusCode < 300) return 'log-success';
                if (log.statusCode >= 400 && log.statusCode < 500) return 'log-warning';
                if (log.statusCode >= 500) return 'log-error';
            }
            return '';
        },
        
        getLogLevelClass(level) {
            switch(level) {
                case 'ERROR': return 'badge-danger';
                case 'WARN': return 'badge-warning';
                case 'INFO': return 'badge-info';
                case 'DEBUG': return 'badge-secondary';
                default: return 'badge-light';
            }
        },
        
        showLogDetails(log) {
            let details = '';
            
            if (log.message) {
                details += `Message:\n${log.message}\n\n`;
            }
            
            if (log.error) {
                details += `Error:\n${log.error}\n\n`;
            }
            
            if (log.requestHeaders) {
                details += `Request Headers:\n${JSON.stringify(log.requestHeaders, null, 2)}\n\n`;
            }
            
            if (log.requestBody) {
                details += `Request Body:\n${log.requestBody}\n\n`;
            }
            
            if (log.responseBody) {
                details += `Response Body:\n${log.responseBody}\n\n`;
            }
            
            if (log.context) {
                details += `Context:\n${JSON.stringify(log.context, null, 2)}`;
            }
            
            alert(details || 'No additional details available');
        },

        showToast(message, type = 'info') {
            this.toast.message = message;
            this.toast.type = type;
            this.toast.show = true;
            setTimeout(() => {
                this.toast.show = false;
            }, 3000);
        }
    },
    beforeUnmount() {
        if (this.autoRefreshInterval) {
            clearInterval(this.autoRefreshInterval);
        }
    }
}).mount('#app');