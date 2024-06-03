package utils

import (
	"crypto/rand"
	"encoding/base64"
	uuid "github.com/satori/go.uuid"
	"io"
	"time"
)

func GetCurrentTimeMs() int64 {
	return time.Now().UnixMilli()
}

func GetRandomToken() (string, error) {
	arr := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, arr); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(arr), nil
}

func GetUUID() string {
	return uuid.NewV4().String()
}
