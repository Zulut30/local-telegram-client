package media

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestMemorySaveGetAndReset(t *testing.T) {
	st := NewMemory()
	ctx := context.Background()
	data := []byte("hello media")

	saved, err := st.Save(ctx, FileInput{
		Kind:        "photo",
		FieldName:   "photo",
		FileName:    "hello.txt",
		ContentType: "text/plain",
		Data:        data,
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if saved.ID == "" || saved.UniqueID == "" || saved.Path == "" {
		t.Fatalf("saved metadata = %#v, want identifiers", saved)
	}

	data[0] = 'j'
	got, err := st.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !bytes.Equal(got.Data, []byte("hello media")) {
		t.Fatalf("stored data = %q, want cloned original", got.Data)
	}

	got.Data[0] = 'x'
	gotAgain, err := st.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("second Get returned error: %v", err)
	}
	if !bytes.Equal(gotAgain.Data, []byte("hello media")) {
		t.Fatalf("stored data after mutation = %q, want isolated clone", gotAgain.Data)
	}

	if err := st.Reset(ctx); err != nil {
		t.Fatalf("Reset returned error: %v", err)
	}
	if _, err := st.Get(ctx, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after reset error = %v, want ErrNotFound", err)
	}
}
