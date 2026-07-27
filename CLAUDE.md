# Viewbook

A model of an app's views: what each screen is for, what it has to do, the states it can be in, and
how it renders today — read, edited and asked from in one place.

Grew out of the Arbay project, where the same thing was tried three times: a generated wiki
(MkDocs), a requirements tool (StrictDoc), and an infinite canvas (Excalidraw as the container).
All three were wrong for the same reason, and the third is the instructive one: a canvas made the
text unselectable and unsearchable, needed a pile of hacks to fit a board to a window, inlined every
thumbnail as base64 into 150 KB scene files, and dirtied git just by being opened. A model is a
document. A sketch is a drawing. Only the second wants a canvas.

## Why it exists

Verified 2026, no product does this:

- **Storybook** is JS/DOM only; it does not touch Kotlin. Every "Storybook for Compose" is an
  unofficial lookalike.
- **Showkase** builds a catalogue *inside the app on a device*. Explicitly not wanted.
- **Roborazzi / Paparazzi** do pixel-diff regression and emit an HTML report; test tools, not a place
  to keep what a screen is meant to do.
- **zeroheight, Supernova, Knapsack, Backlight** document a *Figma file*, not a compiled app.
- **Claude Design** (April 2026) is persistent, reads a codebase and hands off to Claude Code — and
  has zero Android/Compose; its prototypes are web.
- **Android Studio Agent Mode** generates and iterates real Compose from an image or a prompt, but
  the design artifact *is* the source, per file, in the IDE. No inventory, nothing to return to.
- **ComposeProof** (MCP, maturity unverified) gives an agent live UI tree and screenshots of a
  running Compose app. The closest thing to live eyes; worth copying, not depending on.

Nobody ships screen ↔ requirements ↔ live render ↔ conversation as one thing.

## Shape

- `viewbook.go` — the server: config, model read/write, tables, images, sketches, an event stream
  that tells open pages a file changed, `POST api/say`, `GET api/session`.
- `books.go` — several projects, each under its own path, with a list at the root.
- `changes.go` — what someone did, in words ("note on Home: ...", "One count: Broken is now Built"),
  because what reaches a conversation must read as a request.
- `web/` — React. Index of cards, a page per view, config-declared tables, a sketch canvas.
  Built output is committed on purpose: `go:embed` needs it so `go install` works with no npm step.
- `cmd/viewbook` — the standalone binary. `--say` runs a command with the message on stdin.

**[proj](https://github.com/tieo/proj) embeds this package**: `proj viewbook` serves every project
it knows about, `Say` types into that project's terminal session and `Session` reads the reply back
out, so the page shows the answer. That is the only integration, and it lives on proj's side.

## Rules

- **Nothing app-specific, and nothing language-specific.** It models Compose today; it must model a
  web app or an iOS app without changing a line here. Anything project-shaped goes in that project's
  `viewbook.json` or `model.json`.
- **The project's files are the document.** Written whole, moved into place, and written back
  exactly as they arrive — re-encoding sorted every key once, so changing one word rewrote 1035
  lines and the diff said nothing.
- **A document is read, not panned.** The canvas is for sketching, behind its own address.
- **Renders come from the app's own code**, never from drawings.
- **Look at what you produce.** Render the page, read the screenshot, use the thing. A previous
  round shipped a page with no send button, no confirmation and no reply, all of which a single
  honest look would have caught.

## What is next

1. **Beyond Android.** The renders are dropped in `img/` by whatever the project uses. Document and
   prove it with a second, non-Compose project: a web app whose screenshots come from a headless
   browser, or an iOS app from a simulator. Nothing in the tool should need to change; if it does,
   that is the bug to fix.
2. **A glance at the code structure.** Wanted: seeing the broad shape of a codebase from the
   viewbook — which files hold what, how big they are, and especially *where the same thing is done
   twice* (two market lists, two sort orders, a helper copied into three files). The signal to aim
   for is duplication and drift, not a class diagram. Likely: a generated `structure.json` per
   project — modules, files, sizes, exported names, and pairs of near-identical blocks — with the
   same rule as everything else here, that the project generates it and viewbook only shows it.
3. **Adding things from the page.** Status and notes can be edited; adding a requirement, a state or
   a view cannot.
4. **Renders that refresh themselves.** A project should declare the command that produces `img/`,
   and viewbook should be able to run it and pick the result up.
5. **Packaging.** A Nix flake and a release, so `viewbook` installs without a Go toolchain.

## Working agreements

- Prose in files stays full prose. No em dashes, no en dashes.
- Commit messages read like a developer wrote them quickly. No AI attribution.
