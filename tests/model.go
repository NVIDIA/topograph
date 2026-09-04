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
	suffix := ".yaml"
	if strings.HasSuffix(fname, ".yml") {
		suffix = ".yml"
	}
	fname = strings.TrimSuffix(fname, suffix)
	return modelFiles.ReadFile(fmt.Sprintf(MODEL_FILE_PATTERN, fname+suffix))
}
