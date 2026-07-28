# Viewbook

A model of an app's views: what each screen is for, what it has to do, the states it can be in, and
**how it renders today** — in one place you can read, edit and talk from.

Nothing else does this. Storybook does not touch Kotlin. Showkase renders a catalogue inside the app
on a device. zeroheight, Supernova and Knapsack document a Figma file, not a compiled app. Claude
Design is persistent and reads your codebase, but its prototypes are web. Requirements tools know
nothing about rendering. Teams glue those together by hand; this is the glue as one small thing.

## What it gives you

- **An index** of every view, each card carrying that screen's current render.
- **A page per view**: the screenshot beside what the view has to do, each requirement's status
  settable in place, the states it can be in, and notes.
- **A box to ask from.** What you type is handed to whoever works on the app, and the tail of that
  conversation comes back on the page, so asking for a change is not typing into a void.
- **Tables** a project declares itself, from JSON it already has.
- **A sketch canvas** per view, for a screen that does not exist yet.

Everything is a plain file in the project being modelled, so an edit in the browser is a diff in
that project's repository, and whoever works on the app edits exactly what the browser shows.

## Starting a book

    viewbook --init path/to/docs/model      # a config, a model with one view, and img/
    viewbook --gaps path/to/docs/model      # what it is still missing
    viewbook path/to/docs/model             # read it

A view may say where it comes from in the code, and the page shows it:

    "sources": ["src/screens/Home.kt", "src/screens/HomeState.kt"]

## Installing it

    nix run github:tieo/viewbook -- path/to/model          # no checkout, no toolchain
    nix profile install github:tieo/viewbook
    go install github.com/tieo/viewbook/cmd/viewbook@latest

The interface is committed built, and `go:embed` carries it into the binary, so installing needs no
node and no npm step.

## Running it

    viewbook path/to/model
    viewbook --say "proj say myproject" path/to/model
    viewbook --listen 127.0.0.1:8099 one/model two/model

It prints the address with a key in it. Open that once and the browser keeps the key; every request
after that carries it, and one that does not is refused. The key is a file at
`$XDG_STATE_HOME/viewbook/key`, readable by its owner alone, so whoever can read that file is who
can open the book. Requests another site makes on the browser's behalf are refused whatever they
carry, because what is typed here reaches a session that can run commands.

`--say` is the whole integration surface: a command that receives a message on stdin. Point it at a
chat, a session, a webhook, a log. [proj](https://github.com/tieo/proj) implements it as
`proj say <project>`, which types the message into that project's terminal session, and embeds this
package so `proj viewbook` serves every project it knows about.

## A project directory

    viewbook.json    what this book is called, and which tables it carries
    model.json       views, requirements, states, stories
    img/*.png        a render per view; img/small and img/card hold sized copies where a project makes them
    wireframes/      sketches, written by the canvas

A view names its renders, and a view can have more than one: an upright screen and a wide one are
the same view seen on different hardware. Which is which is measured from the image, so the model
declares nothing about shape.

## Every state, not just the happy one

A screen is full and it is empty, it is loading and it has failed, it is permitted and refused. A
book that shows only the happy state lies by omission, and the lie is invisible: a missing empty
state looks exactly like one that cannot happen.

Nothing is required of a view by default. A checklist of Loading, Empty and Failed reported gaps on
screens that have none, and missed every bug anyone actually hit, because the states that matter are
the ones nobody thought to name. What is enforced instead is enforced on the pictures, and needs no
list:

- **Nothing is drawn.** A render that is one colour is a screen that drew nothing.
- **Two states, one picture.** Two renders of the same view, meant to be different states, that look
  the same. Either those are one state, or the screen does not tell them apart, which is what its
  reader will not be able to do either.

Both are reported by `viewbook --gaps` and on the index, without anybody predicting anything. On the
first run across four books they found: a filter panel that renders identically whether its data has
arrived or not, two commands whose empty output is their normal output, and a canvas that renders
blank while it loads.

A state is an entry in `model.json` related to its view by `State of`, and it carries renders of its
own. `viewbook.json` names the states every view is expected to have:

    "states": ["Empty", "Failed"]

A view that has other states says so itself, and a view that has none says that too:

    "states": ["Empty", "Failed", "Cooling down"]   # this view, not the book's list
    "states": []                                    # a static screen: no loading, no failure

A state may be called whatever the app calls it and still count, by naming which required state it
is:

    { "title": "No tags yet", "kind": "Empty", ... }

The page then lists them as chips, each with its own render, and a state nothing renders is listed
anyway, dashed and in the colour of something missing. The index counts them, and says when a view
is missing a required state while carrying one that counts for nothing. So does the command line,
which is how a build is held to it:

    viewbook --gaps path/to/model     # prints "Results: Empty", exits 1 when any are missing

Mind the pipe: `viewbook --gaps model | tail` reports tail's exit status, not viewbook's.

`viewbook.example.json` shows the config. Nothing in the tool knows anything about the app it is
modelling, or what language that app is written in.

## Programs without screens

A book for a command-line program is the same book with a different vocabulary.
Its **views** are what someone looks at: the list a command prints, the screen a
TUI draws, a report. Its **states** are the conditions that output has: nothing
configured yet, nothing found, the thing it talks to is down. Its **shapes** are
terminal widths, not devices:

    "shapes": ["wide", "narrow"]          # 120 and 56 columns, say
    "states": ["Empty", "Failed"]         # a list that prints nothing, and one that cannot

proj models itself this way: five views, ten states, sixty renders taken by
running the real binary against a fabricated home and photographing its output.

A command-line program has one shape, a terminal, and asking it for an upright
render and a wide one would be asking it to lie. A book says which shapes its
screens come in, and is held to those and no others:

    "shapes": ["terminal"]                 # a CLI: one render per state
    "shapes": ["phone", "wide"]            # an app on a phone and a desktop
                                            # left out: nothing is expected

The word is matched against the render's file name or label, so
`split-report-terminal.png` covers `"terminal"`. Views are then whatever someone
actually looks at: a report, a screen of a TUI, the output of one command.
Papercut models two reports and nothing else.

## The renders

Viewbook does not take the screenshots; the project does, and drops them in `img/`. Any toolchain
that can render its own screens will do: an off-screen Compose renderer, a headless browser, a
simulator, a screenshot test. Keeping it that way means the pictures come from the real code and
cannot quietly stop matching it.

A project can say what makes them, and then the page can run it:

    "renders": {
      "command": ["./make-renders.sh"],
      "dir": "../..",
      "statement": "shown next to the button"
    }

`dir` is relative to the model directory, and `env` is what the command needs that this server's
environment does not have:

    "renders": {
      "command": ["./gradlew", "renderGallery"],
      "dir": "../..",
      "env": { "SKIKO_RENDER_API": "SOFTWARE" }
    }

The command's output is on the page while it runs, and when it finishes every open page reloads and
the screenshots it shows carry the moment they changed.

The command runs in the server's environment, not in the shell it was written in, so it must reach
for nothing outside the project's own toolchain: a gradle wrapper, a script in the repository, a
node module the project installs. A crop with ImageMagick works on a laptop and fails under a
service that has never heard of it.
