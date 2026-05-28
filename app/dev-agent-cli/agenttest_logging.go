package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type preparedRunLogger struct {
	*log.Logger
	file *os.File
	path string
}

func newPreparedRunLogger(path, prefix string) (*preparedRunLogger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("log path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &preparedRunLogger{
		Logger: log.New(file, prefix, log.LstdFlags|log.Lmicroseconds),
		file:   file,
		path:   path,
	}, nil
}

func (l *preparedRunLogger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
