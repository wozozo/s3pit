package server

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/wozozo/s3pit/internal/config"
	"github.com/wozozo/s3pit/pkg/api"
	"github.com/wozozo/s3pit/pkg/auth"
	"github.com/wozozo/s3pit/pkg/dashboard"
	"github.com/wozozo/s3pit/pkg/logger"
	"github.com/wozozo/s3pit/pkg/storage"
	"github.com/wozozo/s3pit/pkg/tenant"
)

type Server struct {
	config        *config.Config
	router        *gin.Engine
	storage       storage.Storage
	authHandler   auth.Handler
	tenantManager *tenant.Manager
}

func New(cfg *config.Config) (*Server, error) {
	gin.SetMode(gin.ReleaseMode)

	// Initialize logger
	logInstance := logger.GetInstance()
	if cfg.LogDir != "" {
		logInstance.SetLogDir(cfg.LogDir)
	}
	if cfg.LogLevel != "" {
		switch cfg.LogLevel {
		case "debug":
			logInstance.SetLevel(logger.DEBUG)
		case "info":
			logInstance.SetLevel(logger.INFO)
		case "warn":
			logInstance.SetLevel(logger.WARN)
		case "error":
			logInstance.SetLevel(logger.ERROR)
		}
	}
	logInstance.EnableFileLogging(cfg.EnableFileLog)
	logInstance.EnableConsoleLogging(cfg.EnableConsoleLog)
	logInstance.SetMaxEntries(cfg.MaxLogEntries)

	// Initialize tenant manager first
	tenantMgr := tenant.NewManager(cfg.TenantsFile)
	if cfg.TenantsFile != "" {
		if err := tenantMgr.LoadFromFile(); err != nil {
			log.Printf("Warning: failed to load tenants file: %v", err)
		} else {
			// Update config with dataDir from tenants.json if available
			cfg.UpdateGlobalDirFromTenants(tenantMgr)
		}
	}

	var storageBackend storage.Storage
	var err error

	// Use tenant-aware storage if tenants are configured
	if cfg.TenantsFile != "" && tenantMgr != nil {
		storageBackend = storage.NewTenantAwareStorage(cfg.GlobalDir, tenantMgr, cfg.InMemory)
	} else if cfg.InMemory {
		storageBackend = storage.NewMemoryStorage()
	} else {
		storageBackend, err = storage.NewFileSystemStorage(cfg.GlobalDir)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize storage: %w", err)
		}
	}

	// Use MultiTenantHandler for authentication
	authHandler, err := auth.NewMultiTenantHandler(cfg.AuthMode, tenantMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auth handler: %w", err)
	}

	s := &Server{
		config:        cfg,
		router:        gin.New(),
		storage:       storageBackend,
		authHandler:   authHandler,
		tenantManager: tenantMgr,
	}

	s.setupRoutes()

	return s, nil
}

func (s *Server) setupRoutes() {
	s.router.Use(gin.Recovery())
	s.router.Use(logger.S3APILoggingMiddleware())
	s.router.Use(dashboard.LoggingMiddleware())
	s.router.Use(s.corsMiddleware())
	s.router.Use(s.authMiddleware()) // Add authentication middleware

	// Setup dashboard routes BEFORE S3 API routes to avoid conflicts
	if s.config.EnableDashboard {
		s.setupDashboard()
	}

	apiHandler := api.NewHandler(s.storage, s.authHandler, s.tenantManager, s.config)

	s.router.GET("/", apiHandler.ListBuckets)
	s.router.HEAD("/:bucket", apiHandler.HeadBucket)
	s.router.PUT("/:bucket", apiHandler.CreateBucket)
	s.router.DELETE("/:bucket", apiHandler.DeleteBucket)
	s.router.GET("/:bucket", apiHandler.ListObjectsV2)

	s.router.HEAD("/:bucket/*key", apiHandler.HeadObject)
	s.router.GET("/:bucket/*key", func(c *gin.Context) {
		key := c.Param("key")
		// If key is empty or just "/", this is actually a ListObjectsV2 request
		if key == "" || key == "/" {
			apiHandler.ListObjectsV2(c)
			return
		}
		apiHandler.GetObject(c)
	})
	s.router.PUT("/:bucket/*key", func(c *gin.Context) {
		key := c.Param("key")
		// If key is empty or just "/", this is actually a bucket creation request
		if key == "" || key == "/" {
			apiHandler.CreateBucket(c)
			return
		}

		// Check if this is a copy operation
		if c.GetHeader("x-amz-copy-source") != "" {
			apiHandler.CopyObject(c)
		} else if c.Query("partNumber") != "" && c.Query("uploadId") != "" {
			// This is a part upload for multipart upload
			apiHandler.UploadPart(c)
		} else {
			apiHandler.PutObject(c)
		}
	})
	s.router.DELETE("/:bucket/*key", func(c *gin.Context) {
		// Check if this is an abort multipart upload
		if c.Query("uploadId") != "" {
			apiHandler.AbortMultipartUpload(c)
		} else {
			apiHandler.DeleteObject(c)
		}
	})
	s.router.POST("/:bucket", func(c *gin.Context) {
		// Check if this is a multipart upload operation
		if _, exists := c.GetQuery("uploads"); exists {
			apiHandler.InitiateMultipartUpload(c)
		} else if c.Query("uploadId") != "" {
			apiHandler.CompleteMultipartUpload(c)
		} else {
			apiHandler.DeleteObjects(c)
		}
	})

	s.router.POST("/:bucket/*key", func(c *gin.Context) {
		// Check if this is a multipart upload operation
		if _, exists := c.GetQuery("uploads"); exists {
			apiHandler.InitiateMultipartUpload(c)
		} else if c.Query("uploadId") != "" {
			apiHandler.CompleteMultipartUpload(c)
		} else {
			apiHandler.HandlePostObject(c)
		}
	})
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Expose-Headers", "*")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}

		c.Next()
	}
}

func (s *Server) setupDashboard() {
	// Pass auth configuration to dashboard
	region := s.config.Region
	if region == "" {
		region = "us-east-1"
	}
	dashboardHandler := dashboard.NewHandler(
		s.storage,
		s.tenantManager,
		s.config.AuthMode,
		region,
	)
	dashboardHandler.RegisterRoutes(s.router)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	log.Printf("Starting S3pit server on %s", addr)
	log.Printf("Auth mode: %s", s.config.AuthMode)
	log.Printf("Storage: %s", s.getStorageType())

	// Log tenant information if using tenant manager
	if s.tenantManager != nil && s.config.TenantsFile != "" {
		tenants := s.tenantManager.ListTenants()
		if len(tenants) > 0 {
			log.Printf("Loaded %d tenant(s) from %s", len(tenants), s.config.TenantsFile)
			for _, t := range tenants {
				dir := s.tenantManager.GetDirectory(t.AccessKeyID)
				log.Printf("  - %s: %s", t.AccessKeyID, dir)
			}
		}
	}

	if s.config.AutoCreateBucket {
		log.Printf("Auto-create bucket: enabled")
	}
	if s.config.EnableDashboard {
		log.Printf("Dashboard: http://%s/dashboard", addr)
	}

	return s.router.Run(addr)
}

func (s *Server) getStorageType() string {
	// Check if using tenant-aware storage
	if _, ok := s.storage.(*storage.TenantAwareStorage); ok {
		if s.config.InMemory {
			return "tenant-aware (in-memory)"
		}
		return fmt.Sprintf("tenant-aware filesystem (base: %s)", s.config.GlobalDir)
	}

	if s.config.InMemory {
		return "in-memory"
	}
	return fmt.Sprintf("filesystem (%s)", s.config.GlobalDir)
}
