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

A state is an entry in `model.json` related to its view by `State of`, and it carries renders of its
own. `viewbook.json` names the states every view is expected to have:

    "states": ["Empty", "Failed"]

The page then lists them as chips, each with its own render, and a state nothing renders is listed
anyway, dashed and in the colour of something missing. The index counts them. So does the command
line, which is how a build is held to it:

    viewbook --gaps path/to/model     # prints "Results: Empty", exits 1 when any are missing

`viewbook.example.json` shows the config. Nothing in the tool knows anything about the app it is
modelling, or what language that app is written in.

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

`dir` is relative to the model directory. The command's output is on the page while it runs, and
when it finishes every open page reloads and the screenshots it shows carry the moment they changed.

The command runs in the server's environment, not in the shell it was written in, so it must reach
for nothing outside the project's own toolchain: a gradle wrapper, a script in the repository, a
node module the project installs. A crop with ImageMagick works on a laptop and fails under a
service that has never heard of it.
