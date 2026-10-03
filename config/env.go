package config

import (
	"log"
	"os"
)

// MustGet returns the value of an environment variable or stops the program.
// The values live in .env, which is gitignored (see .env.example).
func MustGet(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("%s is not set (see .env.example)", name)
	}
	return value
}
