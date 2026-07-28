package viewbook

import (
	"fmt"
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

// Measured against real books, for whoever tunes this next. Arbay's phone
// renders are 780 by 5440; a pair of them differing in 12872 pixels is two lines
// of text and must pass, and its wide pair differing in 8300 of 1290 by 2587
// must pass too. Free items differing in 65939, and Results in 20156, are
// different screens and must pass. A copied file, differing in nothing, must be
// caught. At sixty-four cells the first of those measured as identical, which is
// the failure this resolution exists to avoid.
//
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
			if drawable, said := state["drawable"].(bool); said && !drawable {
				continue
			}
			named, _ := state["title"].(string)
			shown[named] = rendersIn(state)
		}

		prints := map[string]uint64{}
		thumbs := map[string][]byte{}
		for state, files := range shown {
			for _, file := range files {
				path := s.renderPath(file)
				flat, print, thumb, err := look(path)
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
				thumbs[state+"\x00"+file] = thumb
			}
		}

		// The same picture under several names is either several names for one
		// state, or a screen that does not tell them apart. Reported once per
		// group rather than once per pair: four states that all look alike are
		// one defect, not six.
		grouped := map[string][]string{} // variant -> keys
		for key := range prints {
			_, file, _ := strings.Cut(key, "\x00")
			grouped[variant(file)] = append(grouped[variant(file)], key)
		}

		// Each variant answers the same question about a different picture, and
		// the answers overlap: the phone renders may show three states alike
		// where the wide ones show four. The widest answer is the finding; a
		// subset of it is the same defect said again.
		type alike struct {
			states []string
			files  []string
			apart  float64
		}
		var groups []alike
		for _, keys := range grouped {
			sort.Strings(keys)
			taken := map[string]bool{}
			for i, key := range keys {
				if taken[key] {
					continue
				}
				stateOne, fileOne, _ := strings.Cut(key, "\x00")
				group := alike{states: []string{stateOne}, files: []string{fileOne}}
				worst := 0.0
				for _, other := range keys[i+1:] {
					stateTwo, fileTwo, _ := strings.Cut(other, "\x00")
					if taken[other] || stateTwo == stateOne {
						continue
					}
					// The hash is no longer a gate. It threw away pairs whose
					// difference was two whole lines of text, because text one
					// character tall does not survive being reduced to a grid of
					// sixty-four cells.
					apartBy := differs(thumbs[key], thumbs[other])
					if apartBy > 0.01 {
						continue
					}
					if apartBy > worst {
						worst = apartBy
					}
					taken[other] = true
					group.states = append(group.states, stateTwo)
					group.files = append(group.files, fileTwo)
				}
				if len(group.states) > 1 {
					sort.Strings(group.states)
					group.apart = worst
					groups = append(groups, group)
				}
			}
		}
		for i, group := range groups {
			widest := true
			for j, other := range groups {
				if i != j && within(group.states, other.states) &&
					(len(group.states) < len(other.states) || (len(group.states) == len(other.states) && i > j)) {
					widest = false
					break
				}
			}
			if !widest {
				continue
			}
			keep(uid+"|same|"+strings.Join(group.states, "\x00"), Finding{
				What: fmt.Sprintf("%s: %s are the same picture", title, list(group.states)),
				Why: fmt.Sprintf(
					"they differ in %.1f%% of what is drawn; states a reader is meant to tell apart look identical, so either the screen does not distinguish them or these renders are of the same thing",
					group.apart*100),
				Files: group.files,
			})
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

// variant is what distinguishes renders of the same state from each other: the
// upright one and the wide one, the light one and the dark one, are not the same
// picture and are not meant to be. Only renders of the same variant are worth
// comparing, and by convention that is the tail of the file name:
// home-empty-phone-dark.png is the phone-dark variant.
func variant(file string) string {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	parts := strings.Split(name, "-")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], "-")
	}
	return name
}

// look reads a render and answers two things: whether anything is drawn on it,
// and a fingerprint that survives small differences.
//
// The fingerprint is a difference hash: the picture reduced to a 9x8 grid of
// brightnesses, each compared with the one to its right. Two pictures that
// differ in a few words differ in a few bits; two that differ in what they show
// differ in dozens.
func look(path string) (flat bool, print uint64, thumb []byte, err error) {
	file, err := os.Open(path)
	if err != nil {
		return false, 0, nil, err
	}
	defer file.Close()
	drawn, err := png.Decode(file)
	if err != nil {
		return false, 0, nil, err
	}

	bounds := drawn.Bounds()
	if bounds.Dx() < 9 || bounds.Dy() < 8 {
		return false, 0, nil, fmt.Errorf("too small to read")
	}

	// A small grey copy settles what the hash only suggests, and it has to be
	// big enough to hold a line of text. At sixty-four cells a phone render is
	// reduced past legibility: two whole lines of difference land inside one
	// cell and disappear, so pictures that differ by a sentence measure as
	// identical. Every cell is the average of the block it stands for, rather
	// than one pixel sampled out of it, so a thin stroke still moves it.
	const side = 256
	thumb = make([]byte, side*side)
	for y := 0; y < side; y++ {
		fromY := bounds.Min.Y + y*bounds.Dy()/side
		toY := bounds.Min.Y + (y+1)*bounds.Dy()/side
		if toY <= fromY {
			toY = fromY + 1
		}
		for x := 0; x < side; x++ {
			fromX := bounds.Min.X + x*bounds.Dx()/side
			toX := bounds.Min.X + (x+1)*bounds.Dx()/side
			if toX <= fromX {
				toX = fromX + 1
			}
			total, count := 0.0, 0
			for at := fromY; at < toY; at++ {
				for across := fromX; across < toX; across++ {
					r, g, b, _ := drawn.At(across, at).RGBA()
					total += (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257
					count++
				}
			}
			thumb[y*side+x] = byte(total / float64(count))
		}
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
	// Flatness is read off the small grey copy, which samples every part of the
	// picture: a grid over the original can step straight past one line of text
	// and call a page blank that is not.
	lowest, highest := thumb[0], thumb[0]
	for _, grey := range thumb {
		if grey < lowest {
			lowest = grey
		}
		if grey > highest {
			highest = grey
		}
	}
	return highest-lowest < 8, print, thumb, nil
}

// differs is how much of what is drawn differs between two renders.
//
// Measured against the whole frame it is meaningless: a terminal is a flat
// background with a few hundred glyphs on it, so two entirely different screens
// differ in two percent of their pixels and two identical ones differ in zero.
// Two percent of a phone screenshot is a rendering artefact; two percent of a
// terminal screenshot is all of its content. So the difference is counted
// against the ink: the parts of either picture that are not its background.
func differs(one, two []byte) float64 {
	if len(one) != len(two) || len(one) == 0 {
		return 1
	}
	// Averaged blocks soften every edge, so what counts as a difference is
	// smaller than a glyph against its background but larger than the noise a
	// renderer leaves behind.
	const apartEnough = 6

	ink := 0
	for _, grey := range []([]byte){one, two} {
		background := modal(grey)
		for _, at := range grey {
			if int(at)-int(background) > apartEnough || int(background)-int(at) > apartEnough {
				ink++
			}
		}
	}
	changed := 0
	for i := range one {
		if int(one[i])-int(two[i]) > apartEnough || int(two[i])-int(one[i]) > apartEnough {
			changed++
		}
	}
	if ink == 0 {
		return 0
	}
	// Both pictures contributed their ink, so the change is measured against
	// the average of the two.
	return float64(changed) / (float64(ink) / 2)
}

// modal is the commonest brightness in a picture, which on any screen worth
// looking at is its background.
func modal(grey []byte) byte {
	var counts [256]int
	for _, at := range grey {
		counts[at]++
	}
	most, which := 0, byte(0)
	for value, count := range counts {
		if count > most {
			most, which = count, byte(value)
		}
	}
	return which
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


// list is a few names in a sentence: "a", "a and b", "a, b and c".
func list(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = fmt.Sprintf("%q", name)
	}
	switch len(quoted) {
	case 0, 1:
		return strings.Join(quoted, "")
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}


// within is whether every name in one group is in the other.
func within(some, all []string) bool {
	has := make(map[string]bool, len(all))
	for _, name := range all {
		has[name] = true
	}
	for _, name := range some {
		if !has[name] {
			return false
		}
	}
	return true
}
