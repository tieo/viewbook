package viewbook

import (
	"fmt"
	"sort"
	"strings"
)

// Gap is one state of one view that nothing renders, or one shape of it that
// nothing draws.
type Gap struct {
	View  string `json:"view"`
	State string `json:"state"`
	Shape string `json:"shape,omitempty"`
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
	declared := s.config()
	shapes := declared.Shapes

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
		gaps = append(gaps, missing(title, "as it is", rendersIn(view), shapes)...)

		// A view may name its own states, and naming none means none: a screen
		// fed synchronously has no loading and no failure, and a book that
		// insists otherwise reports gaps that can never be closed honestly.
		required := declared.States
		if own, ok := statesWanted(view); ok {
			required = own
		}

		drawn := map[string]bool{}
		for _, another := range states {
			state, ok := another.(map[string]any)
			if !ok || !stateOf(state, uid) {
				continue
			}
			named, _ := state["title"].(string)
			drawn[strings.ToLower(named)] = true
			if kind, _ := state["kind"].(string); kind != "" {
				drawn[strings.ToLower(kind)] = true
			}
			// A state can be real and not drawable: a native map layer this
			// renderer stubs out, a snackbar hosted outside the screen, a page
			// the server draws rather than the interface. Deleting it would be a
			// lie about the app; demanding a picture of it would be a lie about
			// the renderer.
			if drawable, said := state["drawable"].(bool); said && !drawable {
				continue
			}
			gaps = append(gaps, missing(title, named, rendersIn(state), shapes)...)
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
		if gaps[i].State != gaps[j].State {
			return gaps[i].State < gaps[j].State
		}
		return gaps[i].Shape < gaps[j].Shape
	})
	return gaps
}

// missing is what a state is short of: a render at all, or a render in each
// shape the book says its screens come in.
func missing(view, state string, files, shapes []string) []Gap {
	if len(files) == 0 {
		return []Gap{{View: view, State: state}}
	}
	var gaps []Gap
	for _, shape := range shapes {
		drawn := false
		for _, file := range files {
			if strings.Contains(strings.ToLower(file), strings.ToLower(shape)) {
				drawn = true
				break
			}
		}
		if !drawn {
			gaps = append(gaps, Gap{View: view, State: state, Shape: shape})
		}
	}
	return gaps
}

// Said is the gaps as lines, for a command that has to report them.
func Said(gaps []Gap) string {
	var out strings.Builder
	for _, gap := range gaps {
		if gap.Shape != "" {
			fmt.Fprintf(&out, "%s: %s, %s\n", gap.View, gap.State, gap.Shape)
			continue
		}
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


// statesWanted is the states a view says it can be in, and whether it said.
// An empty list is an answer: this screen has none.
func statesWanted(view map[string]any) ([]string, bool) {
	listed, ok := view["states"].([]any)
	if !ok {
		return nil, false
	}
	wanted := make([]string, 0, len(listed))
	for _, one := range listed {
		if named, ok := one.(string); ok {
			wanted = append(wanted, named)
		}
	}
	return wanted, true
}
