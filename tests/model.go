package tests

import (
	"embed"
	"fmt"
	"path/filepath"
)

const MODEL_FILE_PATTERN = "models/%s"

//go:embed models/*
var modelFiles embed.FS

// GetModelFileData reads an embedded model by basename; the .yaml suffix is optional.
func GetModelFileData(fname string) ([]byte, error) {
	if filepath.Ext(fname) == "" {
		fname += ".yaml"
	}
	return modelFiles.ReadFile(fmt.Sprintf(MODEL_FILE_PATTERN, fname))
}
