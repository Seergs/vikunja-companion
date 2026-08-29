package config

import (
	"os"

	"github.com/joho/godotenv"
)

// DotenvFile is the local development env file loaded by LoadDotenv.
const DotenvFile = ".env"

// LoadDotenv loads DotenvFile from the working directory into the process
// environment, for local development. Variables already present in the
// environment are left untouched, and a missing file is not an error. Returns
// the path that was loaded, or "" if there was none.
func LoadDotenv() (string, error) {
	if _, err := os.Stat(DotenvFile); err != nil {
		return "", nil
	}
	if err := godotenv.Load(DotenvFile); err != nil {
		return "", err
	}
	return DotenvFile, nil
}
