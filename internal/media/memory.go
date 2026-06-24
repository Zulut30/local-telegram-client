package media

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"sync"
	"unicode"
)

var ErrNotFound = errors.New("media file not found")

type Store interface {
	Save(ctx context.Context, input FileInput) (File, error)
	Get(ctx context.Context, fileID string) (File, error)
	Reset(ctx context.Context) error
}

type FileInput struct {
	Kind        string
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
}

type File struct {
	ID          string
	UniqueID    string
	Path        string
	Kind        string
	FieldName   string
	FileName    string
	ContentType string
	Size        int64
	Data        []byte
}

type Memory struct {
	mu    sync.RWMutex
	files map[string]File
}

func NewMemory() *Memory {
	return &Memory{files: make(map[string]File)}
}

func (m *Memory) Save(_ context.Context, input FileInput) (File, error) {
	if len(input.Data) == 0 {
		return File{}, errors.New("media data is required")
	}
	if input.ContentType == "" {
		input.ContentType = http.DetectContentType(input.Data)
	}
	if input.FileName == "" {
		input.FileName = "upload"
	}
	if input.Kind == "" {
		input.Kind = input.FieldName
	}
	if input.Kind == "" {
		input.Kind = "file"
	}

	data := append([]byte(nil), input.Data...)
	id := fileID(input.Kind, input.FileName, input.ContentType, data)
	file := File{
		ID:          id,
		UniqueID:    id + "_unique",
		Path:        "sim/" + id,
		Kind:        input.Kind,
		FieldName:   input.FieldName,
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Size:        int64(len(data)),
		Data:        data,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files == nil {
		m.files = make(map[string]File)
	}
	m.files[file.ID] = clone(file)
	return file, nil
}

func (m *Memory) Get(_ context.Context, fileID string) (File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	file, ok := m.files[fileID]
	if !ok {
		return File{}, ErrNotFound
	}
	return clone(file), nil
}

func (m *Memory) Reset(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.files = make(map[string]File)
	return nil
}

func fileID(kind, name, contentType string, data []byte) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(name))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(contentType))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	return fmt.Sprintf("%s_%016x", safePrefix(kind), hash.Sum64())
}

func safePrefix(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteByte('_')
		}
		if b.Len() >= 24 {
			break
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "file"
	}
	return out
}

func clone(file File) File {
	file.Data = append([]byte(nil), file.Data...)
	return file
}
