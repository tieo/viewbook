package viewbook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// keptOutput is how much of a run's output is worth holding: enough for the
// failure at the end and the few hundred lines before it, not a whole build log
// nobody reads.
const keptOutput = 96 << 10

// runTimeout is long enough for a Gradle build from cold and short enough that
// a wedged command releases the button.
const runTimeout = 20 * time.Minute

// Renders is how a project says what produces its screenshots: a command in the
// project's own tree, usually a screenshot test over whatever the app declares
// its screens with. Viewbook runs it and shows what came out; what it does is
// entirely the project's business.
type Renders struct {
	Command []string `json:"command"`
	Dir     string   `json:"dir,omitempty"`
	// Env is what the command needs that this server's environment does not
	// have: a library path, a rendering backend, a headless flag. The command
	// runs where the server runs, not in the shell it was written in, and this
	// is where a project says what that costs it.
	Env       map[string]string `json:"env,omitempty"`
	Statement string            `json:"statement,omitempty"`
}

// run is what happened, or is happening, the last time the renders were made.
type run struct {
	sync.Mutex
	running  bool
	output   bytes.Buffer
	started  time.Time
	finished time.Time
	failed   string
	cancel   context.CancelFunc
}

// tail keeps the buffer from growing past what anyone will read, cutting at a
// line so the first line shown is a whole one.
func (r *run) Write(p []byte) (int, error) {
	r.Lock()
	defer r.Unlock()
	n, err := r.output.Write(p)
	if r.output.Len() > keptOutput {
		kept := r.output.Bytes()[r.output.Len()-keptOutput:]
		if cut := bytes.IndexByte(kept, '\n'); cut >= 0 {
			kept = kept[cut+1:]
		}
		next := bytes.NewBuffer(append([]byte(nil), kept...))
		r.output = *next
	}
	return n, err
}

func (s *Server) renders(w http.ResponseWriter, r *http.Request) {
	declared := s.config().Renders
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.renderState(declared))
	case http.MethodPost:
		if declared == nil || len(declared.Command) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error": "this project does not say what makes its renders",
			})
			return
		}
		if !s.startRenders(declared) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already running"})
			return
		}
		writeJSON(w, http.StatusAccepted, s.renderState(declared))
	case http.MethodDelete:
		s.making.Lock()
		stop := s.making.cancel
		s.making.Unlock()
		if stop != nil {
			stop()
		}
		writeJSON(w, http.StatusOK, s.renderState(declared))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// renderState is what a page needs to draw the button and whatever the command
// has said so far.
func (s *Server) renderState(declared *Renders) map[string]any {
	s.making.Lock()
	defer s.making.Unlock()
	state := map[string]any{
		"declared": declared != nil && len(declared.Command) > 0,
		"running":  s.making.running,
		"output":   s.making.output.String(),
		"failed":   s.making.failed,
	}
	if declared != nil {
		state["command"] = strings.Join(declared.Command, " ")
		state["statement"] = declared.Statement
	}
	if !s.making.started.IsZero() {
		state["started"] = s.making.started.Format(time.RFC3339)
	}
	if !s.making.finished.IsZero() {
		state["finished"] = s.making.finished.Format(time.RFC3339)
		state["took"] = s.making.finished.Sub(s.making.started).Round(time.Second).String()
	}
	return state
}

// startRenders runs the declared command in the background, with its output
// readable while it runs: a build that says nothing for four minutes is
// indistinguishable from one that has hung.
func (s *Server) startRenders(declared *Renders) bool {
	s.making.Lock()
	if s.making.running {
		s.making.Unlock()
		return false
	}
	ctx, stop := context.WithTimeout(context.Background(), runTimeout)
	s.making.running = true
	s.making.output.Reset()
	s.making.started = time.Now()
	s.making.finished = time.Time{}
	s.making.failed = ""
	s.making.cancel = stop
	s.making.Unlock()

	command := s.drawing(ctx, declared)
	fmt.Fprintf(&s.making, "%s\nin %s\n\n", strings.Join(declared.Command, " "), command.Dir)

	go func() {
		defer stop()
		command.Stdout = &s.making
		command.Stderr = &s.making
		err := command.Run()
		// A command that is not there fails identically to one that is broken,
		// unless the page is told what was looked for and where. This server's
		// environment is not the shell the command was written in.
		if errors.Is(err, exec.ErrNotFound) {
			fmt.Fprintf(&s.making, "\n%v\nPATH was %s\n", err, os.Getenv("PATH"))
		}

		if left := s.Gaps(); len(left) > 0 {
			fmt.Fprintf(&s.making, "\n%d states nothing renders:\n%s", len(left), Said(left))
		}

		s.making.Lock()
		s.making.running = false
		s.making.finished = time.Now()
		s.making.cancel = nil
		if err != nil {
			s.making.failed = err.Error()
		}
		s.making.Unlock()
		// The renders on disk are what changed, so every open page is told the
		// same way it is told about any other file under the model.
		s.changed()
	}()
	return true
}

// drawing is the declared command ready to run: the project's own directory and
// the environment it says it needs, whether the page's button starts it or a
// terminal does.
func (s *Server) drawing(ctx context.Context, declared *Renders) *exec.Cmd {
	dir := s.Root
	if declared.Dir != "" {
		dir = filepath.Join(s.Root, declared.Dir)
	}
	command := exec.CommandContext(ctx, declared.Command[0], declared.Command[1:]...)
	command.Dir = dir
	command.Env = os.Environ()
	for name, value := range declared.Env {
		command.Env = append(command.Env, name+"="+value)
	}
	return command
}

// Change is one picture that came out different, and by how much.
//
// The size of the difference is the whole signal: two pixels on one row is a
// letter drawn a shade differently, and half the picture is the screen doing
// something else. Without it every rerun reads the same.
type Change struct {
	File   string `json:"file"`
	Pixels int    `json:"pixels"` // -1 when the two cannot be compared pixel for pixel
	Of     int    `json:"of"`
}

// Drawn is what running the project's own command did to the pictures in img/.
type Drawn struct {
	Changed []Change `json:"changed"`
	Added   []string `json:"added"`
	Gone    []string `json:"gone"`
}

// Touched is how many pictures the run was not in agreement with.
//
// Zero is the answer a committed book should give: the pictures on disk are
// what the app draws today. Anything else means the book was describing an app
// that has moved, which is invisible in a way a missing render is not.
func (d Drawn) Touched() int { return len(d.Changed) + len(d.Added) + len(d.Gone) }

// Draw runs the command this project declares and answers what it changed.
//
// A book goes stale silently: every picture is there, every state has one, and
// each shows an app as it was whenever somebody last ran the renders by hand.
// The command is the project's, so this only reports what it did.
//
// What comes back is for a person to read and not for a build to fail on. Two
// machines drawing the same screen do not agree on the pixels: a font stack
// resolves differently, and every box on the page then comes out a different
// size, so a picture can be current and still differ from the one committed. A
// gate on that is red for a reason nobody can fix, and gets deleted along with
// whatever real check sat next to it.
//
// A book whose renders are stable can read any change as a change in the app.
// One where the same few pictures come back every run is drawing something that
// is not the app: a date, a random seed, a page that reads live content, or a
// book that photographs its own pages and so shows its last renders inside its
// next ones. That is worth chasing rather than shrugging at, and it has found a
// real bug here.
func (s *Server) Draw(ctx context.Context, out io.Writer) (Drawn, error) {
	declared := s.config().Renders
	if declared == nil || len(declared.Command) == 0 {
		return Drawn{}, errors.New("this book does not say what makes its renders")
	}
	before, err := s.pictures()
	if err != nil {
		return Drawn{}, err
	}
	// The pictures as they were, kept aside so the ones that come out different
	// can be held against what they were rather than only counted.
	kept, err := s.keepPictures()
	if kept != "" {
		defer os.RemoveAll(kept)
	}
	if err != nil {
		return Drawn{}, err
	}
	command := s.drawing(ctx, declared)
	command.Stdout = out
	command.Stderr = out
	fmt.Fprintf(out, "%s\nin %s\n\n", strings.Join(declared.Command, " "), command.Dir)
	run := command.Run()
	if errors.Is(run, exec.ErrNotFound) {
		fmt.Fprintf(out, "\n%v\nPATH was %s\n", run, os.Getenv("PATH"))
	}
	after, err := s.pictures()
	if err != nil {
		return Drawn{}, err
	}
	drawn := whatChanged(before, after)
	for at, change := range drawn.Changed {
		drawn.Changed[at].Pixels, drawn.Changed[at].Of =
			howDifferent(filepath.Join(kept, change.File), s.path("img", change.File))
	}
	return drawn, run
}

// keepPictures copies img/ somewhere else, so what a run replaced is still
// readable after it has been replaced.
func (s *Server) keepPictures() (string, error) {
	kept, err := os.MkdirTemp("", "viewbook-kept")
	if err != nil {
		return "", err
	}
	root := s.path("img")
	err = filepath.WalkDir(root, func(at string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && at == root {
				return nil
			}
			return err
		}
		name, err := filepath.Rel(root, at)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(kept, name), 0o700)
		}
		body, err := os.ReadFile(at)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(kept, name), body, 0o600)
	})
	return kept, err
}

// howDifferent is how many pixels of two pictures came out different, and how
// many there are. Pictures that cannot be held against each other, because one
// is not an image this can read or they are not the same size, answer -1.
func howDifferent(was, now string) (int, int) {
	one, two := pixels(was), pixels(now)
	if one == nil || two == nil || !one.Bounds().Eq(two.Bounds()) {
		return -1, 0
	}
	box := one.Bounds()
	changed := 0
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			red, green, blue, _ := one.At(x, y).RGBA()
			red2, green2, blue2, _ := two.At(x, y).RGBA()
			if red != red2 || green != green2 || blue != blue2 {
				changed++
			}
		}
	}
	return changed, box.Dx() * box.Dy()
}

func pixels(path string) image.Image {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	drawn, _, err := image.Decode(file)
	if err != nil {
		return nil
	}
	return drawn
}

// pictures is every file under img/ by name, each with a fingerprint of what is
// in it, so a rerun that draws the same thing is not read as a change.
func (s *Server) pictures() (map[string][32]byte, error) {
	held := map[string][32]byte{}
	root := s.path("img")
	err := filepath.WalkDir(root, func(at string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && at == root {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(at)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, at)
		if err != nil {
			return err
		}
		held[name] = sha256.Sum256(body)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return held, nil
}

func whatChanged(before, after map[string][32]byte) Drawn {
	drawn := Drawn{}
	for name, now := range after {
		was, had := before[name]
		switch {
		case !had:
			drawn.Added = append(drawn.Added, name)
		case was != now:
			drawn.Changed = append(drawn.Changed, Change{File: name})
		}
	}
	for name := range before {
		if _, still := after[name]; !still {
			drawn.Gone = append(drawn.Gone, name)
		}
	}
	sort.Slice(drawn.Changed, func(i, j int) bool { return drawn.Changed[i].File < drawn.Changed[j].File })
	sort.Strings(drawn.Added)
	sort.Strings(drawn.Gone)
	return drawn
}
