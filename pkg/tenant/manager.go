package tenant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

type Tenant struct {
	AccessKeyID     string   `toml:"accessKeyId"`
	SecretAccessKey string   `toml:"secretAccessKey"`
	CustomDir       string   `toml:"customDir"`
	Description     string   `toml:"description,omitempty"`
	PublicBuckets   []string `toml:"publicBuckets"` // List of public buckets for this tenant
}

type Config struct {
	GlobalDir string   `toml:"globalDir,omitempty"`
	Tenants   []Tenant `toml:"tenants"`
}

type Manager struct {
	configFile string
	globalDir  string
	tenants    map[string]*Tenant
	mu         sync.RWMutex
}

func NewManager(configFile string) *Manager {
	return &Manager{
		configFile: configFile,
		tenants:    make(map[string]*Tenant),
	}
}

func (m *Manager) LoadFromFile() error {
	if m.configFile == "" {
		return nil
	}

	data, err := os.ReadFile(m.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the global directory from config.toml
	if config.GlobalDir != "" {
		m.globalDir = expandTilde(config.GlobalDir)
	}

	m.tenants = make(map[string]*Tenant)
	for i := range config.Tenants {
		tenant := &config.Tenants[i]
		m.tenants[tenant.AccessKeyID] = tenant
	}

	return nil
}

func (m *Manager) GetTenant(accessKeyID string) (*Tenant, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenant, exists := m.tenants[accessKeyID]
	return tenant, exists
}

func (m *Manager) GetDirectory(accessKeyID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Priority 1: Tenant-specific directory (if specified)
	if tenant, exists := m.tenants[accessKeyID]; exists && tenant.CustomDir != "" {
		return expandTilde(tenant.CustomDir)
	}

	// Priority 2: Global directory + accessKeyID (globalDir is now required)
	return filepath.Join(m.globalDir, accessKeyID)
}

// expandTilde expands the tilde (~) in a path to the user's home directory
func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func (m *Manager) ResolvePath(accessKeyID, bucket, key string) string {
	dir := m.GetDirectory(accessKeyID)

	if key == "" {
		return filepath.Join(dir, bucket)
	}

	return filepath.Join(dir, bucket, key)
}

func (m *Manager) GetAllTenants() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string)
	for accessKey, tenant := range m.tenants {
		result[accessKey] = tenant.CustomDir
	}
	return result
}

// GetGlobalDir returns the global directory from config.toml
func (m *Manager) GetGlobalDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.globalDir
}

// UpdateGlobalDir updates the global directory for tenant storage
func (m *Manager) UpdateGlobalDir(globalDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.globalDir = globalDir
}

// IsPublicBucket checks if a bucket is public for any tenant
func (m *Manager) IsPublicBucket(bucket string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, tenant := range m.tenants {
		for _, publicBucket := range tenant.PublicBuckets {
			if publicBucket == bucket || publicBucket == "*" {
				return true, tenant.AccessKeyID
			}
			// Support wildcard patterns like "public-*"
			if strings.HasSuffix(publicBucket, "*") {
				prefix := strings.TrimSuffix(publicBucket, "*")
				if strings.HasPrefix(bucket, prefix) {
					return true, tenant.AccessKeyID
				}
			}
		}
	}
	return false, ""
}

func (m *Manager) AddTenant(tenant *Tenant) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tenants[tenant.AccessKeyID] = tenant

	if m.configFile != "" {
		return m.saveToFile()
	}

	return nil
}

func (m *Manager) RemoveTenant(accessKeyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tenants, accessKeyID)

	if m.configFile != "" {
		return m.saveToFile()
	}

	return nil
}

func (m *Manager) ListTenants() []*Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := make([]*Tenant, 0, len(m.tenants))
	for _, tenant := range m.tenants {
		tenants = append(tenants, tenant)
	}

	return tenants
}

func (m *Manager) saveToFile() error {
	config := Config{
		GlobalDir: m.globalDir,
		Tenants:   make([]Tenant, 0, len(m.tenants)),
	}

	for _, tenant := range m.tenants {
		config.Tenants = append(config.Tenants, *tenant)
	}

	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(m.configFile, data, 0644)
}
