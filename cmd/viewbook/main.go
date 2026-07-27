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
		server := &viewbook.Server{Root: root, Say: sayWith(*say)}
		go server.Watch(stop)
		books = append(books, viewbook.Book{
			Name:   strings.ToLower(filepath.Base(filepath.Dir(root))),
			Title:  filepath.Base(filepath.Dir(root)),
			Server: server,
		})
	}

	var handler http.Handler = viewbook.Serve(books)
	if len(books) == 1 {
		handler = books[0].Server.Handler("/")
	}
	fmt.Printf("viewbook on http://%s\n", *listen)
	if err := http.ListenAndServe(*listen, handler); err != nil {
		fmt.Fprintln(os.Stderr, "viewbook:", err)
		os.Exit(1)
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
