// Package viewbook serves a project's model: every view of its app, what each
// has to do, the states it can be in, and how each renders today.
//
// The model is plain files in the project's own working tree, so the agent
// working in that project edits exactly what the browser shows, and the browser
// is told the moment a file changes underneath it. Anything typed in the browser
// goes to the session as a message rather than into a channel of its own: the
// conversation has one home, and this is not it.
package viewbook

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed all:web/dist
var site embed.FS

// name is what a sketch may be called, so a request cannot reach outside the
// project's own wireframes directory.
var name = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Server serves one project's model.
type Server struct {
	// Root holds viewbook.json, model.json, img/ and wireframes/.
	Root string
	// Say delivers a message to whoever is working on this project. Nothing here
	// knows how: a tmux session, a log, a webhook are all the same to it.
	Say func(message string) error
	// Session returns what the conversation looks like right now, so a page can
	// show the answer rather than leaving someone wondering whether anything
	// happened. Empty when there is nothing to read.
	Session func() string
	// Wake starts the session that works on this project, and Rest stops it.
	// Opening a book is asking to work on the project, so the page can start
	// what it is going to talk to; stopping it is the same choice made the
	// other way, and both belong where the conversation is rather than in a
	// terminal somewhere else.
	Wake func() error
	Rest func() error

	watchers sync.Map // chan struct{} per subscriber
	prefix   string
	web      fs.FS
	webDir   string // set when the pages come from a directory rather than the binary
	making   run // the command that makes this project's renders, while it runs
}

// Config is what a project says about its own book: what it is called, and
// which tables it carries. Everything app-specific lives here rather than in
// this package.
type Config struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Tables   []Table  `json:"tables"`
	Renders  *Renders `json:"renders,omitempty"`
	// States every view is expected to have a render of, when a project wants a
	// list of its own. Nothing is required by default: a checklist of Loading,
	// Empty and Failed reported gaps on screens that have none and missed every
	// bug anybody actually hit, because the states that matter are the ones
	// nobody thought to name. What is enforced without being named is in
	// checks.go.
	States []string `json:"states,omitempty"`
	// Shapes every render is expected to come in. A phone app is upright and
	// wide; a command-line program is one shape, a terminal, and asking it for
	// two would be asking it to lie. Named here, they are counted as gaps like
	// states are; left out, nothing is expected and one render is enough.
	Shapes []string `json:"shapes,omitempty"`
}

// Table is a list a project already has as JSON, shown as a table. The tool
// never learns what the rows mean.
type Table struct {
	Name      string   `json:"name"`
	Title     string   `json:"title"`
	Source    string   `json:"source"`
	Rows      string   `json:"rows,omitempty"`
	SortBy    string   `json:"sortBy,omitempty"`
	Statement string   `json:"statement,omitempty"`
	Columns   []Column `json:"columns"`
}

type Column struct {
	Field string `json:"field"`
	Title string `json:"title"`
	Yes   string `json:"yes,omitempty"`
	No    string `json:"no,omitempty"`
	Empty string `json:"empty,omitempty"`
}

// Handler is the whole site: the interface, the project's files, and the
// stream that tells a browser one of them changed. Mounted under prefix, which
// is "/" when this book is alone and "/name/" when it is one of several.
func (s *Server) Handler(prefix string) http.Handler {
	if prefix == "" {
		prefix = "/"
	}
	mux := http.NewServeMux()
	mux.HandleFunc(prefix+"api/config", s.getConfig)
	mux.HandleFunc(prefix+"api/model", s.model)
	mux.HandleFunc(prefix+"api/table/", s.table)
	mux.HandleFunc(prefix+"api/sketches", s.sketches)
	mux.HandleFunc(prefix+"api/sketch/", s.sketch)
	mux.HandleFunc(prefix+"api/events", s.events)
	mux.HandleFunc(prefix+"api/say", s.say)
	mux.HandleFunc(prefix+"api/paste", s.paste)
	mux.HandleFunc(prefix+"api/session", s.session)
	mux.HandleFunc(prefix+"api/renders", s.renders)
	mux.HandleFunc(prefix+"api/check", s.check)
	mux.HandleFunc(prefix+"img/", s.image)
	mux.HandleFunc(prefix+"pasted/", s.pasted)
	s.prefix = prefix

	built, from := interfaceFrom(os.Getenv("VIEWBOOK_WEB"))
	s.web, s.webDir = built, from
	files := http.StripPrefix(strings.TrimSuffix(prefix, "/"), http.FileServer(http.FS(built)))
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		// A book mounted at /name must be reached as /name/, or every relative
		// request under it resolves one level too high.
		if prefix != "/" && r.URL.Path == strings.TrimSuffix(prefix, "/") {
			http.Redirect(w, r, prefix, http.StatusMovedPermanently)
			return
		}
		// Anything that is not a file is the interface itself, so /name/view/results
		// is a page rather than a 404. It is served with a base tag naming where
		// this book is mounted: without one, a page two segments deep looks for
		// its own scripts two segments deep and finds nothing.
		within := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, strings.TrimSuffix(prefix, "/")), "/")
		if _, err := fs.Stat(built, within); err != nil || within == "" {
			page, err := fs.ReadFile(built, "index.html")
			if err != nil {
				http.Error(w, "no interface", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write([]byte(strings.Replace(string(page), "<head>",
				`<head><base href="`+prefix+`">`, 1)))
			return
		}
		files.ServeHTTP(w, r)
	})
	return mux
}

// interfaceFrom is where the pages are read from: the copy built into this
// binary, or a directory when VIEWBOOK_WEB names one.
//
// Working on the interface otherwise means rebuilding and redeploying the
// server to see a changed line of CSS, which is minutes of waiting for
// something the browser could have shown on a refresh.
func interfaceFrom(directory string) (fs.FS, string) {
	if directory != "" {
		if _, err := os.Stat(filepath.Join(directory, "index.html")); err == nil {
			return os.DirFS(directory), directory
		}
		fmt.Fprintf(os.Stderr, "viewbook: no index.html in %s, serving the built-in interface\n", directory)
	}
	built, err := fs.Sub(site, "web/dist")
	if err != nil {
		panic(fmt.Sprintf("viewbook: built interface missing: %v", err))
	}
	return built, ""
}

func (s *Server) path(parts ...string) string {
	return filepath.Join(append([]string{s.Root}, parts...)...)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) serveFile(w http.ResponseWriter, path, contentType string) {
	body, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such file"})
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// Config reads the project's own description of its book, with defaults so a
// project that declares nothing still works.
func (s *Server) config() Config {
	cfg := Config{}
	if body, err := os.ReadFile(s.path("viewbook.json")); err == nil {
		_ = json.Unmarshal(body, &cfg)
	}
	if cfg.Title == "" {
		cfg.Title = filepath.Base(s.Root)
	}
	if cfg.Tables == nil {
		cfg.Tables = []Table{}
	}
	return cfg
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	declared := s.config()
	writeJSON(w, http.StatusOK, struct {
		Config
		// Which interface this is. A page that finds a different one has been
		// left behind by a rebuild and reloads itself, rather than waiting to be
		// told by someone wondering why nothing changed.
		Interface string `json:"interface"`
	}{declared, s.interfaceVersion()})
}

func (s *Server) interfaceVersion() string {
	page, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(page))[:12]
}

func (s *Server) model(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.serveFile(w, s.path("model.json"), "application/json")
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unreadable"})
			return
		}
		var incoming map[string]any
		if err := json.Unmarshal(body, &incoming); err != nil || incoming["views"] == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not a model"})
			return
		}
		before := readModel(s.path("model.json"))
		if err := writeWhole(s.path("model.json"), body); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if r.Header.Get("X-Viewbook-Announce") == "1" {
			s.tell(changes(before, readModel(s.path("model.json"))))
		}
		s.changed()
		writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// say carries a message from the page into the conversation. The page is not a
// second inbox: it hands what was typed to the session and shows what came back.
func (s *Server) say(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var asked struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&asked); err != nil || strings.TrimSpace(asked.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing to say"})
		return
	}
	if s.Say == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nowhere to say it"})
		return
	}
	if err := s.Say(asked.Text); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

// paste keeps an image dropped into the page as a file in the project, and
// answers with its path. A conversation can be handed a path; it cannot be
// handed a clipboard.
func (s *Server) paste(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 25<<20))
	if err != nil || len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no image"})
		return
	}
	kind := "png"
	switch r.Header.Get("Content-Type") {
	case "image/jpeg":
		kind = "jpg"
	case "image/webp":
		kind = "webp"
	case "image/gif":
		kind = "gif"
	}
	dir := pasteDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	name := fmt.Sprintf("%s.%s", time.Now().Format("2006-01-02-150405.000"), kind)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path, "url": "pasted/" + name})
}

// pasteDir is where an image dropped into the page is kept: beside the rest of
// the cache, never in the project, because a screenshot pasted to ask a
// question is not part of the model and has no business in its repository.
func pasteDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "viewbook", "pasted")
}

// pasted serves an image back to the page that pasted it.
func (s *Server) pasted(w http.ResponseWriter, r *http.Request) {
	wanted := filepath.Base(filepath.Clean(strings.TrimPrefix(r.URL.Path, s.prefix+"pasted/")))
	s.serveFile(w, filepath.Join(pasteDir(), wanted), "image/png")
}

// session is the tail of the conversation, and whether there is one at all.
// POST starts the session, DELETE stops it.
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if s.Wake == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "nothing here starts a session"})
			return
		}
		if err := s.Wake(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
	case http.MethodDelete:
		if s.Rest == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "nothing here stops a session"})
			return
		}
		if err := s.Rest(); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
	}
	text := ""
	if s.Session != nil {
		text = s.Session()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"text":    text,
		"running": text != "",
		"canWake": s.Wake != nil,
	})
}

func (s *Server) table(w http.ResponseWriter, r *http.Request) {
	wanted := strings.TrimPrefix(r.URL.Path, s.prefix+"api/table/")
	for _, table := range s.config().Tables {
		if table.Name == wanted {
			s.serveFile(w, s.path(table.Source), "application/json")
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such table"})
}

// sketches is every sketch this project holds, so a screen drawn before it
// exists stays findable instead of living at an address someone has to
// remember.
func (s *Server) sketches(w http.ResponseWriter, r *http.Request) {
	drawn := []string{}
	entries, err := os.ReadDir(s.path("wireframes"))
	if err == nil {
		for _, entry := range entries {
			if sketch := strings.TrimSuffix(entry.Name(), ".excalidraw"); sketch != entry.Name() {
				drawn = append(drawn, sketch)
			}
		}
	}
	writeJSON(w, http.StatusOK, drawn)
}

func (s *Server) sketch(w http.ResponseWriter, r *http.Request) {
	sketch := strings.TrimPrefix(r.URL.Path, s.prefix+"api/sketch/")
	if !name.MatchString(sketch) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad name"})
		return
	}
	path := s.path("wireframes", sketch+".excalidraw")
	switch r.Method {
	case http.MethodGet:
		if _, err := os.Stat(path); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"type": "excalidraw", "version": 2, "elements": []any{}, "files": map[string]any{},
			})
			return
		}
		s.serveFile(w, path, "application/json")
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unreadable"})
			return
		}
		var scene map[string]any
		if err := json.Unmarshal(body, &scene); err != nil || scene["type"] != "excalidraw" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not a scene"})
			return
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := writeWhole(path, body); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"saved": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// image serves a render out of the project's img directory.
//
// A project that keeps sized copies in img/small and img/card is served those;
// one that drops a single file in img/ is served that same file for both, so
// having renders at all costs a project nothing but the renders.
func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	wanted := filepath.Clean(strings.TrimPrefix(r.URL.Path, s.prefix+"img/"))
	path := s.path("img", wanted)
	if !strings.HasPrefix(path, s.path("img")+string(os.PathSeparator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "outside"})
		return
	}
	if _, err := os.Stat(path); err != nil {
		if sized := filepath.Dir(wanted); sized == "small" || sized == "card" {
			path = s.path("img", filepath.Base(wanted))
		}
	}
	s.serveFile(w, path, "image/png")
}

// check is what is wrong with this book: a model that will not parse, and every
// state nothing renders. A page can then say it in the page, which is where
// someone is looking when they find out.
func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	trouble := []map[string]string{}
	body, err := os.ReadFile(s.path("model.json"))
	if err != nil {
		trouble = append(trouble, map[string]string{
			"what": "model.json cannot be read", "why": err.Error(),
		})
	} else {
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			trouble = append(trouble, map[string]string{
				"what": "model.json is not valid JSON", "why": err.Error(),
			})
		} else {
			for _, kind := range []string{"views", "requirements", "states", "stories"} {
				if _, ok := parsed[kind]; !ok {
					trouble = append(trouble, map[string]string{
						"what": "model.json has no " + kind,
						"why":  "a model is four lists: views, requirements, states and stories",
					})
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trouble":  trouble,
		"gaps":     s.Gaps(),
		"hints":    s.hints(),
		"findings": s.Findings(),
	})
}

// hints are the near misses: a view missing a required state while carrying a
// state of its own that nothing else accounts for. Calling an empty state "No
// tags" is reasonable and silently does not count, which is the worst way for a
// contract to be enforced.
func (s *Server) hints() []string {
	model := readModel(s.path("model.json"))
	views, _ := model["views"].([]any)
	states, _ := model["states"].([]any)
	required := s.config().States

	said := []string{}
	for _, one := range views {
		view, ok := one.(map[string]any)
		if !ok {
			continue
		}
		uid, _ := view["uid"].(string)
		title, _ := view["title"].(string)
		wanted := required
		if own, ok := statesWanted(view); ok {
			wanted = own
		}

		named := map[string]bool{}
		spare := []string{}
		for _, another := range states {
			state, ok := another.(map[string]any)
			if !ok || !stateOf(state, uid) {
				continue
			}
			has, _ := state["title"].(string)
			kind, _ := state["kind"].(string)
			named[strings.ToLower(has)] = true
			if kind != "" {
				named[strings.ToLower(kind)] = true
				continue
			}
			spare = append(spare, has)
		}
		for _, want := range wanted {
			if named[strings.ToLower(want)] || len(spare) == 0 {
				continue
			}
			said = append(said, fmt.Sprintf(
				"%s has no state called %q, and has %q, which counts for nothing. Either rename it, or give it \"kind\": %q.",
				title, want, spare[0], want))
		}
	}
	return said
}

// tell hands each change to whoever is working on this project.
func (s *Server) tell(said []string) {
	if s.Say == nil || len(said) == 0 {
		return
	}
	message := "From the viewbook:\n" + strings.Join(said, "\n")
	if err := s.Say(message); err != nil {
		fmt.Fprintf(os.Stderr, "viewbook: could not pass on the change: %v\n", err)
	}
}

// writeWhole replaces a file in one move, so a reader never sees half of one.
//
// The body is written exactly as it arrived. Re-encoding it here would sort every
// object's keys, so changing one word rewrote the whole file and the diff said
// nothing about what was done; the sender formats it instead.
func writeWhole(path string, body []byte) error {
	if len(body) == 0 || body[len(body)-1] != '\n' {
		body = append(body, '\n')
	}
	temporary := path + ".writing"
	if err := os.WriteFile(temporary, body, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func readModel(path string) map[string]any {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var model map[string]any
	if err := json.Unmarshal(body, &model); err != nil {
		return nil
	}
	return model
}

// Watch tells every open browser when a file under the project changes, so a
// page shows what the agent just wrote without anyone reloading it.
func (s *Server) Watch(stop <-chan struct{}) {
	var last map[string]time.Time
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			now := s.stamps()
			if last != nil && !sameStamps(last, now) {
				s.changed()
			}
			last = now
		}
	}
}

func (s *Server) stamps() map[string]time.Time {
	stamps := map[string]time.Time{}
	watched := []string{s.path("model.json"), s.path("img"), s.path("wireframes")}
	// A rebuilt interface is a change the open pages need to hear about too,
	// otherwise they keep running the one they were loaded with.
	if s.webDir != "" {
		watched = append(watched, filepath.Join(s.webDir, "index.html"))
	}
	for _, where := range watched {
		_ = filepath.WalkDir(where, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if info, err := entry.Info(); err == nil {
				stamps[path] = info.ModTime()
			}
			return nil
		})
	}
	return stamps
}

func sameStamps(a, b map[string]time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for path, when := range a {
		if other, ok := b[path]; !ok || !other.Equal(when) {
			return false
		}
	}
	return true
}

func (s *Server) changed() {
	s.watchers.Range(func(key, _ any) bool {
		select {
		case key.(chan struct{}) <- struct{}{}:
		default: // a browser already has one waiting; one nudge is enough
		}
		return true
	})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming here", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	mine := make(chan struct{}, 1)
	s.watchers.Store(mine, true)
	defer s.watchers.Delete(mine)

	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-mine:
			fmt.Fprint(w, "event: changed\ndata: {}\n\n")
			flusher.Flush()
		case <-time.After(25 * time.Second):
			fmt.Fprint(w, ": still here\n\n")
			flusher.Flush()
		}
	}
}
