package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

const sampleSize = 64 * 1024

func Full(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Quick(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if size <= sampleSize*2 {
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
	} else {
		if _, err := io.CopyN(h, f, sampleSize); err != nil {
			return "", err
		}
		if _, err := f.Seek(-sampleSize, io.SeekEnd); err != nil {
			return "", err
		}
		if _, err := io.CopyN(h, f, sampleSize); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
