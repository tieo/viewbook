package viewbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A book on disk, because Gaps reads the project's files and part of what it
// answers is whether a declared render is one of them.
func book(t *testing.T, model string, drawn ...string) *Server {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("viewbook.json", `{"title": "Book"}`)
	write("model.json", model)
	for _, file := range drawn {
		write(filepath.Join("img", file), "not really a picture")
	}
	return &Server{Root: root}
}

const oneView = `{
  "views": [{"uid": "VIEW-LIST", "title": "List"}],
  "requirements": [],
  "states": [%s],
  "stories": []
}`

func state(role, to, renders string) string {
	return `{"uid": "STATE-A", "title": "Empty", "renders": [` + renders +
		`], "relations": [{"to": "` + to + `", "role": "` + role + `"}]}`
}

// A role is written by hand, so the same role written another way is the same
// role. It failing silently reads as a screen with no states rather than as a
// typo.
func TestRoleSpellingIsTheWritersOwn(t *testing.T) {
	for _, role := range []string{"State of", "state_of", "state-of", "STATE OF"} {
		model := strings.Replace(oneView, "%s", state(role, "VIEW-LIST", `"list-empty.png"`), 1)
		gaps := book(t, model, "list-empty.png").Gaps()
		if len(gaps) != 0 {
			t.Errorf("role %q: %d gaps, wanted none: %v", role, len(gaps), gaps)
		}
	}
}

func TestGapsSayWhy(t *testing.T) {
	for _, one := range []struct {
		name  string
		state string
		drawn []string
		view  string
		why   string
	}{
		{"no render", state("State of", "VIEW-LIST", ""), nil, "List", whyNothing},
		{"render not on disk", state("State of", "VIEW-LIST", `"gone.png"`), nil, "List", whyNotThere},
		{"role nobody knows", state("belongs to", "VIEW-LIST", `"list-empty.png"`),
			[]string{"list-empty.png"}, "", `its relation role "belongs to"`},
		{"view nobody has", state("State of", "VIEW-NOPE", `"list-empty.png"`),
			[]string{"list-empty.png"}, "", `which is no view's uid`},
	} {
		t.Run(one.name, func(t *testing.T) {
			model := strings.Replace(oneView, "%s", one.state, 1)
			gaps := book(t, model, one.drawn...).Gaps()
			found := false
			for _, gap := range gaps {
				if gap.View == one.view && strings.Contains(gap.Why, one.why) {
					found = true
				}
			}
			if !found {
				t.Errorf("wanted a gap on view %q saying %q, got %v", one.view, one.why, gaps)
			}
		})
	}
}

// A state of no view is reported on its own. Printed under a view, it says the
// relation was read, which is what sends a reader looking at the renders.
func TestALooseStateIsUnderNoView(t *testing.T) {
	model := strings.Replace(oneView, "%s", state("belongs to", "VIEW-LIST", `"list-empty.png"`), 1)
	for _, gap := range book(t, model, "list-empty.png").Gaps() {
		if gap.State == "Empty" && gap.View != "" {
			t.Errorf("a state of no view was printed under %q", gap.View)
		}
	}
}
