package index

import (
	"bufio"
	"encoding/json"
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
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var files []domain.FileInstance
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for s.Scan() {
		var file domain.FileInstance
		if err := json.Unmarshal(s.Bytes(), &file); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, s.Err()
}
