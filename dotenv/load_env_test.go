package dotenv

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

type envPair struct {
	key   string
	value string
}

type testFixture struct {
	filename       string
	expectedOutput []envPair
}

var fixtures = []testFixture{
	{"../fixtures/env/simple_env", []envPair{{"ENV", "dev"}}},
	{"../fixtures/env/env_with_equal", []envPair{{"MONGO-URI", "mongodb://username@password=dummy/test"}, {"ACCESS_DEV", "909-090"}}},
	{"../fixtures/env/env_with_spaces", []envPair{{"MONGO-URI", "mongodb://username@password=dummy/test"}}},
}

func TestLoadEnvFromString(t *testing.T) {
	os.Clearenv()

	t.Cleanup(func() { os.Clearenv() }) // Clear environment variables after test

	cwd, _ := os.Getwd()
	fmt.Println(cwd)

	for _, fixture := range fixtures {
		err := LoadEnv(fixture.filename)
		assert.NoError(t, err)

		for _, pair := range fixture.expectedOutput {
			assert.Equal(t, os.Getenv(pair.key), pair.value)
		}
	}
}

func TestNoEnvFile(t *testing.T) {
	err := LoadEnv()
	assert.NoError(t, err)
}

func TestLoadEnv_ShouldPickUpEnvFromCWD(t *testing.T) {
	os.Clearenv()
	tmp := t.TempDir()

	// work inside an isolated dir so relative "db" / "services" paths are safe
	orig, _ := os.Getwd()
	t.Cleanup(func() {
		_ = os.Chdir(orig)
		os.Clearenv()
	})
	_ = os.Chdir(tmp)

	envContent := `
MONGO-URI=mongodb://username@password=dummy/test
ACCESS_DEV=909-090
`
	envFile := ".env"
	err := os.WriteFile(envFile, []byte(envContent), 0644)
	assert.NoError(t, err)

	err = LoadEnv(envFile)
	assert.NoError(t, err)

	expected := map[string]string{
		"MONGO-URI":  "mongodb://username@password=dummy/test",
		"ACCESS_DEV": "909-090",
	}

	for key, value := range expected {
		assert.Equal(t, value, os.Getenv(key), "Environment variable %s should match", key)
	}
}
