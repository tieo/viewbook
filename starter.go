package viewbook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Start writes a book that works into dir: a config, a model with one view in
// it, and the directory its renders go in.
//
// The alternative was reading a README and hand-authoring JSON against a schema
// nobody had written down, which is a fair description of no path at all from
// nothing to a book.
func Start(dir, title string) error {
	if _, err := os.Stat(filepath.Join(dir, "model.json")); err == nil {
		return fmt.Errorf("%s already has a model.json", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o755); err != nil {
		return err
	}

	// No renders command is written: one naming a script that does not exist
	// promises a button that fails the first time it is pressed. What draws this
	// project's screens is the project's to connect, and the next-steps text
	// says how.
	config := map[string]any{
		"title":  title,
		"states": []string{"Loading", "Empty", "Failed"},
	}
	model := map[string]any{
		"views": []any{map[string]any{
			"tag":       "VIEW",
			"uid":       "VIEW-EXAMPLE",
			"title":     "An example screen",
			"statement": "What this screen is for, in one sentence. Delete this view once there is a real one.",
			"status":    "Built",
			"relations": []any{},
			"sources":   []string{},
			"renders":   []any{},
		}},
		"requirements": []any{map[string]any{
			"tag":       "REQ",
			"uid":       "REQ-EXAMPLE",
			"title":     "Something this screen has to do",
			"statement": "What has to be true of it, in one sentence.",
			"status":    "Missing",
			"relations": []any{map[string]any{"to": "VIEW-EXAMPLE", "role": "Lives in"}},
		}},
		"states":  []any{},
		"stories": []any{},
	}

	for name, body := range map[string]any{"viewbook.json": config, "model.json": model} {
		written, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(written, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}
