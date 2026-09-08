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

# Two servers: one with nothing working on the project, which is what most of
# these screens look like, and one with a conversation in it for the state that
# is about having one. Giving every render a session made four states of the
# view page the same picture, which the checks then reported, correctly.
viewbook --listen "127.0.0.1:$port" --key-file "$work/key" \
  "$here" "$work/example/docs/model" "$work/bare/docs/model" >"$work/serve.log" 2>&1 &
server=$!

talking=$((port + 1))
viewbook --listen "127.0.0.1:$talking" --key-file "$work/key" --session-file "$work/session.txt" \
  "$here" >"$work/talking.log" 2>&1 &
talker=$!
trap 'rm -rf "$work"; kill "$server" "$talker" 2>/dev/null || true' EXIT
: > "$work/drawn"

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
# The same photograph, taken against another server.
shoot_on() {
  local at=$1 file=$2 where=$3 width=$4 height=$5 theme=$6
  rm -rf "$work/profile-$file"
  chromium --headless --disable-gpu --no-sandbox \
    --user-data-dir="$work/profile-$file" \
    --virtual-time-budget=7000 --run-all-compositor-stages-before-draw \
    --window-size="$width,$height" \
    --screenshot="$work/$file.png" \
    "http://127.0.0.1:$at$where?key=$key&static=1&theme=$theme" >/dev/null 2>&1 || true
  if [ -s "$work/$file.png" ]; then
    mv "$work/$file.png" "$here/img/$file.png"
    echo "$file.png" >> "$work/drawn"
    echo "  $file"
  else
    echo "$file" >> "$work/missed"
    echo "  $file: nothing was written" >&2
  fi
}

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
    echo "$file.png" >> "$work/drawn"
    echo "  $file"
  else
    echo "$file" >> "$work/missed"
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
    shoot "$file-wide-$theme" "$path" "$hash" 1180 760 "$theme"
    shoot "$file-tall-$theme" "$path" "$hash" 430 932 "$theme"
  done
done <<'VIEWS'
index BOOK #/
view BOOK #/view/table
table BOOK #/table/endpoints
sketch BOOK #/sketch/scratch
books / -
VIEWS

# The states a screen here can be in, which the page will show on demand so they
# can be photographed: waiting for the model, holding nothing, and failing to
# read it. A book that demands these of every project draws its own.
# Each page is drawn in the states the model says it has and in no others: the
# list of books cannot be forced into a state at all, a sketch is never empty,
# and drawing those anyway leaves pictures in img/ that nothing in the book
# names.
echo "rendering the states"
for theme in light dark; do
  while read -r name path states; do
    [ -z "$name" ] && continue
    for state in $states; do
      shoot "$name-$state-wide-$theme" "$book" "&showing=$state$path" 1180 760 "$theme"
      shoot "$name-$state-tall-$theme" "$book" "&showing=$state$path" 430 932 "$theme"
    done
  done <<'STATES'
index #/ loading empty failed
view #/view/table loading empty failed attached
table #/table/endpoints loading empty failed
sketch #/sketch/scratch loading failed
STATES

  # A book whose views have no renders, and a view with a conversation in it.
  shoot "index-bare-wide-$theme" "/bare/" "#/" 1180 760 "$theme"
  shoot "index-bare-tall-$theme" "/bare/" "#/" 430 932 "$theme"
  shoot_on "$talking" "view-talking-wide-$theme" "/view/table" 1180 760 "$theme"
  shoot_on "$talking" "view-talking-tall-$theme" "/view/table" 430 932 "$theme"
done

# A picture this run did not draw is a picture of a screen that is no longer in
# the book, and it stays behind looking as current as the rest.
#
# The sweep is valid only because the list above is complete: this script draws
# the whole book every time, so what it did not write is what the book no longer
# has. A run that drew part of it would delete the rest. A run that failed to
# take a shot sweeps nothing, since the file it would remove is the good picture
# still on disk, and a run that drew nothing at all sweeps nothing either.
if [ -s "${work}/missed" ]; then
  echo "$(wc -l < "$work/missed") shots failed, so nothing was swept" >&2
elif [ ! -s "$work/drawn" ]; then
  # Nothing drawn and nothing reported as missed is a run that did not happen.
  # Sweeping against an empty list would take the whole book with it.
  echo "nothing was drawn, so nothing was swept" >&2
else
  swept=0
  for held in "$here"/img/*.png; do
    [ -e "$held" ] || continue
    if ! grep -qxF "$(basename "$held")" "$work/drawn"; then
      rm "$held"
      echo "  swept $(basename "$held")"
      swept=$((swept + 1))
    fi
  done
  if [ "$swept" -eq 1 ]; then
    echo "1 picture nothing draws any more was removed"
  elif [ "$swept" -gt 1 ]; then
    echo "$swept pictures nothing draws any more were removed"
  fi
fi

echo "done"
