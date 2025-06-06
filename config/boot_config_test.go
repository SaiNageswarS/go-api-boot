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
	CustomField string `ini:"custom_field"`
}

func TestLoadConfig_LoadsFromIni(t *testing.T) {
	// Step 1: Create temporary .ini config file
	iniContent := `
ssl_bucket = bucket_ini
domain = example.com
azure_storage_account = mystorageaccount
azure_key_vault_name = mysecretvault
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
	assert.Equal(t, "mysecretvault", cfg.AzureKeyVaultName)
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

func TestLoadConfig_ConfigObjectIsNil(t *testing.T) {
	// Step 1: Attempt to load config with nil target
	var nilConfig *AppConfig = nil
	err := LoadConfig("does_not_matter.ini", nilConfig)
	assert.Error(t, err)
	assert.Equal(t, "target cannot be nil", err.Error())
}

func TestLoadConfig_EmptyIniFile(t *testing.T) {
	// Step 1: Create temporary empty .ini config file
	tmpFile := filepath.Join(t.TempDir(), "empty.ini")
	err := os.WriteFile(tmpFile, []byte(""), 0644)
	assert.NoError(t, err)

	// Step 2: Load config
	var cfg AppConfig
	err = LoadConfig(tmpFile, &cfg)
	assert.NoError(t, err)

	// Step 3: Validate default values
	assert.Equal(t, "", cfg.SslBucket)
	assert.Equal(t, "", cfg.Domain)
	assert.Equal(t, "", cfg.AzureStorageAccount)
	assert.Equal(t, "", cfg.CustomField)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// Step 1: Attempt to load config from a non-existent file
	var cfg AppConfig
	err := LoadConfig("non_existent.ini", &cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")
}
