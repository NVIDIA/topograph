package tests

import (
	"embed"
	"fmt"
	"strings"
)

const MODEL_FILE_PATTERN = "models/%s"

//go:embed models/*
var modelFiles embed.FS

func GetModelFileData(fname string) ([]byte, error) {
	fname = strings.TrimSuffix(fname, ".yaml")
	fname = strings.TrimSuffix(fname, ".yml")
	return modelFiles.ReadFile(fmt.Sprintf(MODEL_FILE_PATTERN, fname+".yaml"))
}
