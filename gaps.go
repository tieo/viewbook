package viewbook

import (
	"fmt"
	"sort"
	"strings"
)

// Gap is one state of one view that nothing renders.
type Gap struct {
	View  string `json:"view"`
	State string `json:"state"`
}

// Gaps is every state a book says a screen can be in that no render shows.
//
// A book whose pictures are all of the happy path is a book that lies by
// omission, and the lie is invisible: the missing empty state looks exactly
// like a state that cannot happen. This counts them, so a project can be held
// to its own list rather than to whoever remembers.
func (s *Server) Gaps() []Gap {
	model := readModel(s.path("model.json"))
	views, _ := model["views"].([]any)
	states, _ := model["states"].([]any)
	required := s.config().States

	var gaps []Gap
	for _, one := range views {
		view, ok := one.(map[string]any)
		if !ok {
			continue
		}
		uid, _ := view["uid"].(string)
		title, _ := view["title"].(string)
		if title == "" {
			title = uid
		}
		if len(rendersIn(view)) == 0 {
			gaps = append(gaps, Gap{View: title, State: "as it is"})
		}

		drawn := map[string]bool{}
		for _, another := range states {
			state, ok := another.(map[string]any)
			if !ok || !stateOf(state, uid) {
				continue
			}
			named, _ := state["title"].(string)
			if len(rendersIn(state)) == 0 {
				gaps = append(gaps, Gap{View: title, State: named})
			}
			drawn[strings.ToLower(named)] = true
		}
		// A state the config asks every view to have, which this view does not
		// even model, is the same gap one step earlier.
		for _, wanted := range required {
			if !drawn[strings.ToLower(wanted)] {
				gaps = append(gaps, Gap{View: title, State: wanted})
			}
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].View != gaps[j].View {
			return gaps[i].View < gaps[j].View
		}
		return gaps[i].State < gaps[j].State
	})
	return gaps
}

// Said is the gaps as lines, for a command that has to report them.
func Said(gaps []Gap) string {
	var out strings.Builder
	for _, gap := range gaps {
		fmt.Fprintf(&out, "%s: %s\n", gap.View, gap.State)
	}
	return out.String()
}

func stateOf(state map[string]any, uid string) bool {
	relations, _ := state["relations"].([]any)
	for _, one := range relations {
		relation, ok := one.(map[string]any)
		if ok && relation["to"] == uid && relation["role"] == "State of" {
			return true
		}
	}
	return false
}

// rendersIn is what an entry says renders it: a list, or the single screenshot
// a model written before there were lists still carries.
func rendersIn(entry map[string]any) []string {
	var files []string
	if listed, ok := entry["renders"].([]any); ok {
		for _, one := range listed {
			switch shot := one.(type) {
			case string:
				files = append(files, shot)
			case map[string]any:
				if file, ok := shot["file"].(string); ok {
					files = append(files, file)
				}
			}
		}
	}
	if file, ok := entry["screenshot"].(string); ok && file != "" {
		files = append(files, file)
	}
	return files
}
