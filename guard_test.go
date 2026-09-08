package viewbook

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Two servers sharing a key file is the ordinary setup: proj serves every
// project's book from one key, and this book's own renderer starts a second
// server beside the first. Both making a key means one of them refuses every
// browser that read the file, which is what a render of the refusal page in
// place of the page being photographed looks like.
func TestOneKeyHoweverManyAskAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "viewbook.key")
	keys := make([]string, 8)
	start := make(chan struct{})
	var together sync.WaitGroup
	for at := range keys {
		together.Add(1)
		go func() {
			defer together.Done()
			<-start
			key, err := KeyAt(path)
			if err != nil {
				t.Error(err)
				return
			}
			keys[at] = key
		}()
	}
	close(start)
	together.Wait()
	for _, key := range keys {
		if key != keys[0] {
			t.Fatalf("keys differ: %q and %q", keys[0], key)
		}
	}
	if held, ok := keyIn(path); !ok || held != keys[0] {
		t.Fatalf("the file holds %q, everybody was told %q", held, keys[0])
	}
}

// An empty key file is a file somebody made, not a server halfway through
// writing one, so it gets filled rather than refused.
func TestAnEmptyKeyFileIsFilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "viewbook.key")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := KeyAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if held, ok := keyIn(path); !ok || held != key {
		t.Fatalf("the file holds %q, the caller was told %q", held, key)
	}
}
