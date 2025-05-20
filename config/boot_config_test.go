package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AppConfig is a test struct that embeds BaseConfig
type AppConfig struct {
	BootConfig  `ini:",extends"`
	CustomField string `env:"CUSTOM-FIELD" ini:"custom_field"`
}

func TestLoadConfig_LoadsFromIniAndEnv(t *testing.T) {
	// Step 1: Create temporary .ini config file
	iniContent := `
mongo_uri = mongodb://localhost:27017
ssl_bucket = bucket_ini
domain = example.com
azure_storage_account = mystorageaccount
custom_field = from_ini
`
	tmpFile := filepath.Join(t.TempDir(), "test.ini")
	err := os.WriteFile(tmpFile, []byte(iniContent), 0644)
	assert.NoError(t, err)

	// Step 2: Set environment variable overrides
	os.Setenv("ACCESS-SECRET", "from_env")
	os.Setenv("CUSTOM-FIELD", "env_value")
	defer os.Clearenv() // clean up env after test

	// Step 3: Load config
	var cfg AppConfig
	err = LoadConfig(tmpFile, &cfg)
	assert.NoError(t, err)

	// Step 4: Validate values
	assert.Equal(t, "mongodb://localhost:27017", cfg.MongoUri)
	assert.Equal(t, "bucket_ini", cfg.SslBucket)
	assert.Equal(t, "example.com", cfg.Domain)
	assert.Equal(t, "mystorageaccount", cfg.AzureStorageAccount)
	assert.Equal(t, "env_value", cfg.CustomField) // overridden by env
}
