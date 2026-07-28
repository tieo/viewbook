#!/usr/bin/env bash
# Makes this book's renders: viewbook, photographed by a headless browser.
#
# Everything a project needs to produce img/ lives here rather than in the tool,
# which is the point: a Compose app runs a screenshot test, a web app runs a
# browser, and viewbook only runs whatever the project declares.
#
# It serves a copy of this model on a spare port, takes each view in both shapes,
# and puts the files where the model says its renders are.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
port="${VIEWBOOK_RENDER_PORT:-8131}"
work="$(mktemp -d)"
trap 'rm -rf "$work"; [ -n "${server:-}" ] && kill "$server" 2>/dev/null || true' EXIT

# A second book, so the list of books has something to be a list of. It is this
# same model under another name, which is all that view needs to show.
mkdir -p "$work/example/docs"
cp -r "$here" "$work/example/docs/model"

# A copy with its renders taken away, so the card that says nothing renders this
# yet can be photographed saying it.
mkdir -p "$work/bare/docs"
cp -r "$here" "$work/bare/docs/model"
rm -f "$work/bare/docs/model/img/"*.png

# A conversation to show in the states that have one. Nothing summons a real
# session for a screenshot.
cat > "$work/session.txt" <<'TALK'
> About Results: the price column is cut off on a phone

  Reading src/main.jsx, then the two rules that size that column.

* Working (12s)
TALK

viewbook --listen "127.0.0.1:$port" --key-file "$work/key" --session-file "$work/session.txt" \
  "$here" "$work/example/docs/model" "$work/bare/docs/model" >"$work/serve.log" 2>&1 &
server=$!

for _ in $(seq 30); do
  [ -s "$work/key" ] && curl -sf -o /dev/null "http://127.0.0.1:$port/" && break
  sleep 0.2
done
key="$(cat "$work/key")"

# A fresh profile per shot: chromium reuses a running instance otherwise and
# writes nothing. --static keeps the page from holding a stream open, which is
# what the screenshot tool waits on.
# Every screen is drawn twice, once in each theme, because a page read in the
# dark should not be illustrated with a picture of the light one.
shoot() {
  local file=$1 path=$2 hash=$3 width=$4 height=$5 theme=${6:-}
  chromium --headless --disable-gpu --no-sandbox \
    --user-data-dir="$work/profile-$file" \
    --virtual-time-budget=7000 --run-all-compositor-stages-before-draw \
    --window-size="$width,$height" \
    --screenshot="$work/$file.png" \
    "http://127.0.0.1:$port$path?key=$key&static=1${theme:+&theme=$theme}$hash" >/dev/null 2>&1 || true
  if [ -s "$work/$file.png" ]; then
    mv "$work/$file.png" "$here/img/$file.png"
    echo "  $file"
  else
    echo "  $file: nothing was written" >&2
    return 1
  fi
}

# Books are served under the project's own name, which is the directory holding
# docs/model.
book="/$(basename "$(dirname "$(dirname "$here")")" | tr '[:upper:]' '[:lower:]')/"

echo "rendering viewbook, in both shapes and both themes"
while read -r file path hash; do
  [ -z "$file" ] && continue
  [ "$path" = "BOOK" ] && path="$book"
  [ "$hash" = "-" ] && hash=""
  for theme in light dark; do
    shoot "$file-wide-$theme" "$path" "$hash" 1440 900 "$theme"
    shoot "$file-tall-$theme" "$path" "$hash" 430 932 "$theme"
  done
done <<'VIEWS'
index BOOK #/
view BOOK #/view/index
table BOOK #/table/endpoints
sketch BOOK #/sketch/scratch
books / -
VIEWS

# The states a screen here can be in, which the page will show on demand so they
# can be photographed: waiting for the model, holding nothing, and failing to
# read it. A book that demands these of every project draws its own.
echo "rendering the states"
for theme in light dark; do
  for state in loading empty failed; do
    for page in "index:#/" "view:#/view/index" "table:#/table/endpoints" "sketch:#/sketch/scratch"; do
      name="${page%%:*}"
      hash="${page#*:}"
      shoot "$name-$state-wide-$theme" "$book" "&showing=$state$hash" 1440 900 "$theme"
      shoot "$name-$state-tall-$theme" "$book" "&showing=$state$hash" 430 932 "$theme"
    done
    shoot "books-$state-wide-$theme" "/" "&showing=$state" 1440 900 "$theme"
    shoot "books-$state-tall-$theme" "/" "&showing=$state" 430 932 "$theme"
  done

  # A book whose views have no renders, and a view with a conversation in it.
  shoot "index-bare-wide-$theme" "/bare/" "#/" 1440 900 "$theme"
  shoot "index-bare-tall-$theme" "/bare/" "#/" 430 932 "$theme"
  shoot "view-talking-wide-$theme" "$book" "#/view/view" 1440 900 "$theme"
  shoot "view-talking-tall-$theme" "$book" "#/view/view" 430 932 "$theme"
done

echo "done"
