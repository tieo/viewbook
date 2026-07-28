package viewbook

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A finding is something wrong with a render that nobody had to predict.
//
// Naming the states a screen can be in is guesswork, and the bugs that have
// actually been found here were not on anybody's list: a filter whose options
// arrived with the data, a failed state that looks exactly like an empty one, a
// list that prints nothing at all. None of those is a missing state. Each is a
// property of the pictures themselves, and a property can be checked without
// being predicted.
type Finding struct {
	What  string `json:"what"`
	Why   string `json:"why"`
	Files []string `json:"files"`
}

// Findings is what the renders say about themselves.
//
//	Nothing is drawn      a render that is one colour is a screen that failed to
//	                      draw, or a state whose whole content is missing.
//	Two states, one look  two renders of the same view, meant to be different
//	                      states, that are the same picture. Either the states
//	                      are the same state, or the screen does not distinguish
//	                      them - which is what a reader will not be able to do
//	                      either.
func (s *Server) Findings() []Finding {
	model := readModel(s.path("model.json"))
	views, _ := model["views"].([]any)
	states, _ := model["states"].([]any)

	var found []Finding
	// One finding per thing wrong, not one per picture of it: the same two
	// states look the same in every shape and every theme they are drawn in.
	said := map[string]int{}
	keep := func(key string, finding Finding) {
		if at, seen := said[key]; seen {
			found[at].Files = append(found[at].Files, finding.Files...)
			return
		}
		said[key] = len(found)
		found = append(found, finding)
	}

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

		// Every picture of this view, with the state it is meant to show.
		shown := map[string][]string{} // state -> files
		shown["as it is"] = rendersIn(view)
		for _, another := range states {
			state, ok := another.(map[string]any)
			if !ok || !stateOf(state, uid) {
				continue
			}
			named, _ := state["title"].(string)
			shown[named] = rendersIn(state)
		}

		prints := map[string]uint64{}
		for state, files := range shown {
			for _, file := range files {
				path := s.renderPath(file)
				flat, print, err := look(path)
				if err != nil {
					continue
				}
				if flat {
					keep(uid+"|blank|"+state, Finding{
						What:  fmt.Sprintf("%s, %s: nothing is drawn", title, state),
						Why:   "the whole render is one colour, so either the screen drew nothing or its content is missing",
						Files: []string{file},
					})
					continue
				}
				prints[state+"\x00"+file] = print
			}
		}

		// The same picture under two names is either two names for one state or
		// a screen that does not tell them apart.
		keys := make([]string, 0, len(prints))
		for key := range prints {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				first, second := keys[i], keys[j]
				stateOne, fileOne, _ := strings.Cut(first, "\x00")
				stateTwo, fileTwo, _ := strings.Cut(second, "\x00")
				if stateOne == stateTwo || shape(fileOne) != shape(fileTwo) {
					continue
				}
				if apart(prints[first], prints[second]) > 3 {
					continue
				}
				pair := []string{stateOne, stateTwo}
				sort.Strings(pair)
				keep(uid+"|same|"+pair[0]+"|"+pair[1], Finding{
					What:  fmt.Sprintf("%s: %q and %q are the same picture", title, pair[0], pair[1]),
					Why:   "two states a reader is meant to tell apart look identical; either the screen does not distinguish them, or one of these renders is of the wrong state",
					Files: []string{fileOne, fileTwo},
				})
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].What < found[j].What })
	return found
}

// renderPath is where a named render actually is: img/, or the sized copies a
// project keeps beside it.
func (s *Server) renderPath(file string) string {
	direct := s.path("img", file)
	if _, err := os.Stat(direct); err == nil {
		return direct
	}
	return s.path("img", "small", file)
}

// shape is what distinguishes renders of the same state from each other: the
// upright one and the wide one are not the same picture and are not meant to be.
func shape(file string) string {
	return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))[strings.LastIndex(
		strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)), "-")+1:]
}

// look reads a render and answers two things: whether anything is drawn on it,
// and a fingerprint that survives small differences.
//
// The fingerprint is a difference hash: the picture reduced to a 9x8 grid of
// brightnesses, each compared with the one to its right. Two pictures that
// differ in a few words differ in a few bits; two that differ in what they show
// differ in dozens.
func look(path string) (flat bool, print uint64, err error) {
	file, err := os.Open(path)
	if err != nil {
		return false, 0, err
	}
	defer file.Close()
	drawn, err := png.Decode(file)
	if err != nil {
		return false, 0, err
	}

	bounds := drawn.Bounds()
	if bounds.Dx() < 9 || bounds.Dy() < 8 {
		return false, 0, fmt.Errorf("too small to read")
	}

	grid := make([][]float64, 8)
	for y := range grid {
		grid[y] = make([]float64, 9)
		for x := range grid[y] {
			at := drawn.At(
				bounds.Min.X+x*bounds.Dx()/9+bounds.Dx()/18,
				bounds.Min.Y+y*bounds.Dy()/8+bounds.Dy()/16,
			)
			r, g, b, _ := at.RGBA()
			grid[y][x] = 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
		}
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			print <<= 1
			if grid[y][x] > grid[y][x+1] {
				print |= 1
			}
		}
	}
	return sameThroughout(drawn), print, nil
}

// sameThroughout is whether a picture is one colour: sampled rather than read
// whole, since a render that is drawing anything at all differs within the
// first few dozen samples.
func sameThroughout(drawn image.Image) bool {
	bounds := drawn.Bounds()
	first := drawn.At(bounds.Min.X, bounds.Min.Y)
	firstR, firstG, firstB, _ := first.RGBA()
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			at := drawn.At(
				bounds.Min.X+x*bounds.Dx()/40,
				bounds.Min.Y+y*bounds.Dy()/40,
			)
			r, g, b, _ := at.RGBA()
			if r != firstR || g != firstG || b != firstB {
				return false
			}
		}
	}
	return true
}

// apart is how many bits two fingerprints differ by.
func apart(one, two uint64) int {
	diff := one ^ two
	count := 0
	for diff != 0 {
		count += int(diff & 1)
		diff >>= 1
	}
	return count
}
