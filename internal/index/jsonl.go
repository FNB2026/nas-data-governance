package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"nas-data-governance/internal/domain"
)

func Write(path string, files []domain.FileInstance) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, file := range files {
		if err := json.NewEncoder(w).Encode(file); err != nil {
			return err
		}
	}
	return w.Flush()
}

func Read(path string) ([]domain.FileInstance, error) {
	var files []domain.FileInstance
	err := Walk(path, func(file domain.FileInstance) error {
		files = append(files, file)
		return nil
	})
	return files, err
}

// Walk streams one JSONL record at a time without retaining the whole index.
func Walk(path string, visit func(domain.FileInstance) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 2*1024*1024)
	line := 0
	for s.Scan() {
		line++
		var file domain.FileInstance
		if err := json.Unmarshal(s.Bytes(), &file); err != nil {
			return fmt.Errorf("index line %d: %w", line, err)
		}
		if err := visit(file); err != nil {
			return fmt.Errorf("index line %d: %w", line, err)
		}
	}
	return s.Err()
}
