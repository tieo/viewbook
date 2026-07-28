package viewbook

import (
	"fmt"
	"html"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// keyCookie is where a browser keeps the key once it has been let in.
const keyCookie = "viewbook"

// Guard stands in front of a viewbook.
//
// A page here types into the session working on a project, so reaching this
// server is reaching the machine. Two things therefore hold for every request.
//
// It is same-origin. A page on any other site can make a browser send a request
// here, and a request that arrives from one carries the same cookies as one the
// user made; without this check, opening the wrong web page is enough to type a
// command into a session.
//
// It carries the key. The key is a file only its owner can read, so a process
// that reaches the port - through the tunnel, from another account, from
// anything that gets as far as the socket - still cannot say anything.
//
// An empty key turns the second check off, which is for a book with no session
// behind it and nothing to say into.
func Guard(key string, next http.Handler) http.Handler {
	return GuardFrom(key, "", next)
}

// GuardFrom is Guard, told where the key is kept so it can say so when a
// request arrives without one.
func GuardFrom(key, keyFile string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		if given := r.URL.Query().Get("key"); given != "" {
			if subtle.ConstantTimeCompare([]byte(given), []byte(key)) != 1 {
				http.Error(w, "wrong key", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     keyCookie,
				Value:    key,
				Path:     "/",
				HttpOnly: true,
				Secure:   overTLS(r),
				SameSite: http.SameSiteStrictMode,
				MaxAge:   365 * 24 * 60 * 60,
			})
			// The key belongs in the cookie jar, not in the address bar, the
			// history, or the referrer of every link the page carries.
			clean := *r.URL
			query := clean.Query()
			query.Del("key")
			clean.RawQuery = query.Encode()
			http.Redirect(w, r, clean.RequestURI(), http.StatusSeeOther)
			return
		}
		if !carriesKey(r, key) {
			refused(w, keyFile)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin is whether the request came from this book rather than from
// another site that happens to be open in the same browser.
//
// What has to be refused is a request another site makes on the browser's
// behalf: a form post, a fetch, an image that is really a command. Following a
// link here is none of those - arriving from a login portal or from a chat
// window is how anyone gets here at all - so a plain navigation is let through
// and judged by the key.
//
// Fetch metadata says where a request came from and every current browser sends
// it; Origin covers a form post, which is the one cross-site request that
// carries no preflight. A request with neither is not from a browser - curl, a
// health check, the screenshot tool - and the key alone decides.
func sameOrigin(r *http.Request) bool {
	reading := r.Method == http.MethodGet || r.Method == http.MethodHead
	if reading && r.Header.Get("Sec-Fetch-Mode") == "navigate" {
		return true
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && !reading {
		from, err := url.Parse(origin)
		if err != nil || from.Host != r.Host {
			return false
		}
	}
	return true
}

func carriesKey(r *http.Request, key string) bool {
	given := r.Header.Get("X-Viewbook-Key")
	if given == "" {
		if cookie, err := r.Cookie(keyCookie); err == nil {
			given = cookie.Value
		}
	}
	return subtle.ConstantTimeCompare([]byte(given), []byte(key)) == 1
}

func overTLS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// KeyAt is the key kept at path, made the first time it is asked for. The file
// is the owner's alone, so who can read it is who can open the book.
func KeyAt(path string) (string, error) {
	if body, err := os.ReadFile(path); err == nil {
		if key := strings.TrimSpace(string(body)); key != "" {
			return key, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", err
	}
	return key, nil
}


// refused is what someone sees who reached the door without the key. A blank
// page with one sentence on it tells them they are locked out and nothing
// about how to get in; this says where the key is and what to do with it.
func refused(w http.ResponseWriter, keyFile string) {
	where := keyFile
	if where == "" {
		where = "the file the server printed on startup"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Viewbook needs its key</title><style>
:root{color-scheme:light dark;--bg:#fbfbf9;--panel:#fff;--ink:#14181a;--quiet:#6b7478;--line:#e4e6e3;--accent:#2f9e44}
@media(prefers-color-scheme:dark){:root{--bg:#14171a;--panel:#1b1f22;--ink:#e9ecef;--quiet:#99a2a8;--line:#2b3136}}
body{margin:0;background:var(--bg);color:var(--ink);font-family:-apple-system,"Segoe UI",system-ui,sans-serif;
display:flex;min-height:100vh;align-items:center;justify-content:center}
main{width:min(620px,90vw);padding:40px 0}h1{font-size:22px;margin:0 0 10px}
p{color:var(--quiet);font-size:14px;line-height:1.6;margin:0 0 14px}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:13px;background:var(--panel);
border:1px solid var(--line);border-radius:7px;padding:2px 6px}
pre{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:12px 14px;overflow:auto;
font-size:13px;color:var(--ink)}
</style></head><body><main>
<h1>Viewbook needs its key</h1>
<p>What is typed in a viewbook reaches a session that can run commands, so the page opens for
whoever holds the key and for nobody else.</p>
<p>The key is a file only its owner can read:</p>
<pre>%s</pre>
<p>Open the book with it once and the browser keeps it:</p>
<pre>%s?key=$(cat %s)</pre>
<p>The server prints that address on startup. If this is someone else's machine, that is the
answer: there is nothing here for you.</p>
</main></body></html>`, html.EscapeString(where), "http://127.0.0.1:8099/", html.EscapeString(where))
}
