package viewbook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

	dir := s.Root
	if declared.Dir != "" {
		dir = filepath.Join(s.Root, declared.Dir)
	}
	fmt.Fprintf(&s.making, "%s\nin %s\n\n", strings.Join(declared.Command, " "), dir)

	go func() {
		defer stop()
		command := exec.CommandContext(ctx, declared.Command[0], declared.Command[1:]...)
		command.Dir = dir
		command.Env = os.Environ()
		for name, value := range declared.Env {
			command.Env = append(command.Env, name+"="+value)
		}
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
