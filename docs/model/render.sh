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

viewbook --listen "127.0.0.1:$port" --key-file "$work/key" "$here" "$work/example/docs/model" \
  >"$work/serve.log" 2>&1 &
server=$!

for _ in $(seq 30); do
  [ -s "$work/key" ] && curl -sf -o /dev/null "http://127.0.0.1:$port/" && break
  sleep 0.2
done
key="$(cat "$work/key")"

# A fresh profile per shot: chromium reuses a running instance otherwise and
# writes nothing. --static keeps the page from holding a stream open, which is
# what the screenshot tool waits on.
shoot() {
  local file=$1 path=$2 hash=$3 width=$4 height=$5
  chromium --headless --disable-gpu --no-sandbox \
    --user-data-dir="$work/profile-$file" \
    --virtual-time-budget=7000 --run-all-compositor-stages-before-draw \
    --window-size="$width,$height" \
    --screenshot="$work/$file.png" \
    "http://127.0.0.1:$port$path?key=$key&static=1$hash" >/dev/null 2>&1 || true
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

echo "rendering viewbook, in both shapes"
while read -r file path hash; do
  [ -z "$file" ] && continue
  [ "$path" = "BOOK" ] && path="$book"
  [ "$hash" = "-" ] && hash=""
  shoot "$file-wide" "$path" "$hash" 1440 900
  shoot "$file-tall" "$path" "$hash" 430 932
done <<'VIEWS'
index BOOK #/
view BOOK #/view/index
table BOOK #/table/endpoints
sketch BOOK #/sketch/scratch
books / -
VIEWS

echo "done"
