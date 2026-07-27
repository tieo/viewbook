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

## Running it

    viewbook path/to/model
    viewbook --say "proj say myproject" path/to/model
    viewbook --listen 127.0.0.1:8099 one/model two/model

`--say` is the whole integration surface: a command that receives a message on stdin. Point it at a
chat, a session, a webhook, a log. [proj](https://github.com/tieo/proj) implements it as
`proj say <project>`, which types the message into that project's terminal session, and embeds this
package so `proj viewbook` serves every project it knows about.

## A project directory

    viewbook.json    what this book is called, and which tables it carries
    model.json       views, requirements, states, stories
    img/small/*.png  a render per view
    img/card/*.png   the same renders cropped for the index
    wireframes/      sketches, written by the canvas

`viewbook.example.json` shows the config. Nothing in the tool knows anything about the app it is
modelling, or what language that app is written in.

## The renders

Viewbook does not take the screenshots; the project does, and drops them in `img/`. Any toolchain
that can render its own screens will do: an off-screen Compose renderer, a headless browser, a
simulator, a screenshot test. Keeping it that way means the pictures come from the real code and
cannot quietly stop matching it.
