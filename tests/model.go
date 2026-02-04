package tests

import (
	"embed"
	"fmt"
)

const MODEL_FILE_PATTERN = "models/%s"

//go:embed models/*
var modelFiles embed.FS

func GetModelFileData(fname string) ([]byte, error) {
	return modelFiles.ReadFile(fmt.Sprintf(MODEL_FILE_PATTERN, fname))
}
