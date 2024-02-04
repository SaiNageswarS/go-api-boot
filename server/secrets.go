package server

import (
	"github.com/joho/godotenv"
)

func LoadSecretsIntoEnv() {
	godotenv.Load()
}
