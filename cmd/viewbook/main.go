// Command viewbook serves a model of an app's views: what each screen is for,
// what it has to do, the states it can be in, and how each renders today.
//
// It takes one project directory, or several, and serves each under its own
// path. Anything typed in the browser is handed to a command of your choosing,
// so the tool never needs to know how you talk to whoever works on the app.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tieo/viewbook"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8099", "address to serve on")
	say := flag.String("say", "", "command run with a message on stdin when something is changed or asked (e.g. \"proj say myproject\")")
	sessionFile := flag.String("session-file", "",
		"file whose contents are shown as the conversation; for rendering the states that have one")
	start := flag.Bool("init", false, "write a book that works into the given directory and exit")
	gaps := flag.Bool("gaps", false, "list the states nothing renders and exit non-zero when there are any")
	keyFile := flag.String("key-file", defaultKeyPath(),
		"file holding the key the browser must carry; empty serves to anyone who reaches the port")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `viewbook - a model of an app's views

  viewbook [flags] <project-dir> [project-dir...]

A project directory holds viewbook.json, model.json, img/ and wireframes/.
Given one it is served at the root; given several, each is served under its own
name, with a list of them at the root.

`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	if *start {
		for _, dir := range flag.Args() {
			root, err := filepath.Abs(dir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "viewbook:", err)
				os.Exit(1)
			}
			if err := viewbook.Start(root, projectName(root)); err != nil {
				fmt.Fprintln(os.Stderr, "viewbook:", err)
				os.Exit(1)
			}
			fmt.Printf(`wrote a book into %s

  viewbook.json   what it is called and which states every view must have
  model.json      one example view and one requirement, to replace with real ones
  img/            where the renders go

Next:
  1. Replace the example view with this project's screens, one entry each.
  2. Draw them however this project can - a screenshot test, a headless browser,
     an off-screen renderer - into img/, named so the shape and the theme are in
     the file name: home-phone-dark.png.
  3. List them on the view, and on each state, as
     {"file": "home-phone-dark.png", "label": "phone dark"}.
  4. Say what draws them, and the page gets a button that runs it:
     "renders": {"command": ["./make-renders.sh"], "dir": "../.."}
     A command-line program says so too: "shapes": ["terminal"]

  viewbook --gaps %s   what is still missing
  viewbook %s          read it
`, root, root, root)
		}
		return
	}

	// Held to its own list: a project's build can run this and fail on a screen
	// whose empty or failed state nobody has ever drawn.
	if *gaps {
		found := 0
		for _, dir := range flag.Args() {
			root, err := filepath.Abs(dir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "viewbook:", err)
				os.Exit(1)
			}
			server := &viewbook.Server{Root: root}
			missing := server.Gaps()
			found += len(missing)
			if said := viewbook.Said(missing); said != "" {
				fmt.Print(said)
			}
		}
		if found > 0 {
			fmt.Fprintf(os.Stderr, "%d states nothing renders\n", found)
			os.Exit(1)
		}
		fmt.Println("every state has a render")
		return
	}

	var books []viewbook.Book
	stop := make(chan struct{})
	for _, dir := range flag.Args() {
		root, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "viewbook:", err)
			os.Exit(1)
		}
		if _, err := os.Stat(filepath.Join(root, "model.json")); err != nil {
			fmt.Fprintf(os.Stderr, "viewbook: no model.json in %s\n", root)
			os.Exit(1)
		}
		server := &viewbook.Server{Root: root, Say: sayWith(*say), Session: sessionFrom(*sessionFile)}
		go server.Watch(stop)
		title := projectName(root)
		books = append(books, viewbook.Book{
			Name:   strings.ToLower(title),
			Title:  title,
			Server: server,
		})
	}

	var handler http.Handler = viewbook.Serve(books)
	if len(books) == 1 {
		handler = books[0].Server.Handler("/")
	}

	// What is typed here reaches whoever is working on the project, and with
	// --say that is a command on this machine. The key is what keeps the page
	// to the person who started it.
	key := ""
	if *keyFile != "" {
		var err error
		if key, err = viewbook.KeyAt(*keyFile); err != nil {
			fmt.Fprintln(os.Stderr, "viewbook:", err)
			os.Exit(1)
		}
	}
	opening := "http://" + *listen + "/"
	if key != "" {
		opening += "?key=" + key
	}
	fmt.Printf("viewbook on %s\n", opening)
	if err := http.ListenAndServe(*listen, viewbook.GuardFrom(key, *keyFile, handler)); err != nil {
		fmt.Fprintln(os.Stderr, "viewbook:", err)
		os.Exit(1)
	}
}

// projectName is what to call a book kept at a path like project/docs/model:
// the project, rather than the folder it happens to be filed under, which is
// the same word in every project and cannot tell two of them apart.
func projectName(root string) string {
	filed := map[string]bool{"model": true, "models": true, "docs": true, "doc": true}
	for at := filepath.Clean(root); ; {
		name := filepath.Base(at)
		up := filepath.Dir(at)
		if up == at || name == "" {
			return filepath.Base(filepath.Clean(root))
		}
		if !filed[strings.ToLower(name)] {
			return name
		}
		at = up
	}
}

// defaultKeyPath is where the key that opens the books is kept: with the rest
// of this machine's state, readable by its owner alone.
func defaultKeyPath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "viewbook", "key")
}

// sessionFrom reads the conversation from a file, which is how a project draws
// the states of a screen that has one: a real session cannot be summoned for a
// screenshot, and a screen showing a conversation is a state like any other.
func sessionFrom(path string) func() string {
	if path == "" {
		return nil
	}
	return func() string {
		body, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return string(body)
	}
}

// sayWith runs the configured command with the message on stdin. Whether that
// reaches a person, a session or a log is the command's business, not this
// program's.
func sayWith(command string) func(string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	parts := strings.Fields(command)
	return func(message string) error {
		run := exec.Command(parts[0], parts[1:]...)
		run.Stdin = strings.NewReader(message)
		run.Stderr = os.Stderr
		return run.Run()
	}
}
