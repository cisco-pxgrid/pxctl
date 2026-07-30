package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONObjectStreamCountsAndDecodesIncrementally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	input := `{"metadata":{"name":"test"},"records":[{"id":1},{"id":2},{"id":3}]}`
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}

	count, err := countJSONObjects(path)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 objects, got %d", count)
	}

	stream, err := newJSONObjectStream(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for expected := 1; expected <= 3; expected++ {
		object, done, err := stream.Next()
		if err != nil || done {
			t.Fatalf("unexpected stream result at %d: object=%v done=%t err=%v", expected, object, done, err)
		}
		if object["id"] != float64(expected) {
			t.Fatalf("expected id %d, got %v", expected, object["id"])
		}
	}
	_, done, err := stream.Next()
	if err != nil || !done {
		t.Fatalf("expected end of stream, done=%t err=%v", done, err)
	}
}
