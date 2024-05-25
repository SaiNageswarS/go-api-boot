package server

import (
	"os"
	"strings"
)

// LoadEnv loads environment variables from .env file.
func LoadEnv(envPath ...string) error {
	if len(envPath) == 0 {
		envPath = append(envPath, ".env")
	}

	for _, filename := range envPath {
		content, err := os.ReadFile(filename)
		if err != nil {
			return err
		}

		err = LoadEnvFromString(string(content))
		if err != nil {
			return err
		}
	}

	return nil
}

func LoadEnvFromString(env string) error {
	lines := strings.Split(env, "\n")
	for _, line := range lines {
		// skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "=")
		key := strings.TrimSpace(parts[0])

		// join rest of the parts with "="
		value := strings.TrimSpace(strings.Join(parts[1:], "="))

		os.Setenv(key, value)
	}

	return nil
}
