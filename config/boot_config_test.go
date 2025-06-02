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

func TestLoadConfig_LoadsFromIni(t *testing.T) {
	// Step 1: Create temporary .ini config file
	iniContent := `
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
	defer os.Clearenv() // clean up env after test

	// Step 3: Load config
	var cfg AppConfig
	err = LoadConfig(tmpFile, &cfg)
	assert.NoError(t, err)

	// Step 4: Validate values
	assert.Equal(t, "bucket_ini", cfg.SslBucket)
	assert.Equal(t, "example.com", cfg.Domain)
	assert.Equal(t, "mystorageaccount", cfg.AzureStorageAccount)
}

func TestLoadConfig_LoadsBasedOnEnv(t *testing.T) {
	// Step 1: Create temporary .ini config file
	iniContent := `
[dev]
ssl_bucket = bucket_ini
domain = localhost
azure_storage_account = mystorageaccount
custom_field = from_ini

[prod]
ssl_bucket = bucket_ini
domain = api.temporal.com
azure_storage_account = mystorageaccount
custom_field = prod_ini
`
	tmpFile := filepath.Join(t.TempDir(), "test.ini")
	err := os.WriteFile(tmpFile, []byte(iniContent), 0644)
	assert.NoError(t, err)

	defer os.Clearenv() // clean up env after test
	// Step 2: Set environment variable overrides
	os.Setenv("ENV", "dev")

	// Step 3: Load config
	var cfg AppConfig
	err = LoadConfig(tmpFile, &cfg)
	assert.NoError(t, err)

	// Step 4: Validate values
	assert.Equal(t, "bucket_ini", cfg.SslBucket)
	assert.Equal(t, "localhost", cfg.Domain)
	assert.Equal(t, "mystorageaccount", cfg.AzureStorageAccount)
	assert.Equal(t, "from_ini", cfg.CustomField)

	// Step 5: Change RUN_MODE to prod and reload
	os.Setenv("ENV", "prod")
	err = LoadConfig(tmpFile, &cfg)
	assert.NoError(t, err)

	// Step 6: Validate prod values
	assert.Equal(t, "bucket_ini", cfg.SslBucket)
	assert.Equal(t, "api.temporal.com", cfg.Domain)
	assert.Equal(t, "mystorageaccount", cfg.AzureStorageAccount)
	assert.Equal(t, "prod_ini", cfg.CustomField)
}
