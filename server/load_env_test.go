package server

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
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
	cwd, _ := os.Getwd()
	fmt.Println(cwd)

	for _, fixture := range fixtures {
		err := LoadEnv(fixture.filename)
		require.NoError(t, err)

		for _, pair := range fixture.expectedOutput {
			require.Equal(t, os.Getenv(pair.key), pair.value)
		}
	}
}
