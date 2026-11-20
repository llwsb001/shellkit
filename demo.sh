#!/bin/sh
# demo.sh — one core, three shells, one notes file. Run: ./demo.sh (make demo).
set -eu
GO=${GO:-/home/user/tools/go1.27.4/bin/go}
BIN=./bin/shellkit
PORT=${SHELLKIT_ADDR:-127.0.0.1:3447}
export SHELLKIT_HOME=$(mktemp -d)

echo "── capture from the CLI"
$GO build -trimpath -o "$BIN" .
"$BIN" add -t demo "the page and the terminal read the same file"
"$BIN" add -t demo "no framework, no bundler, three static files"
"$BIN" list

echo
echo "── the same data over HTTP (gateway on $PORT)"
SHELLKIT_ADDR=$PORT "$BIN" web >/tmp/shellkit-demo.log 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT

i=0
while [ $i -lt 40 ]; do
  if curl -fsS "http://$PORT/healthz" >/dev/null 2>&1; then break; fi
  i=$((i + 1)); sleep 0.15
done

echo "   POST /api/notes   →"
curl -sS -X POST "http://$PORT/api/notes" -H 'content-type: application/json' \
  -d '{"text":"added from curl","tags":["demo"]}'
echo "   GET  /api/notes   →"
curl -sS "http://$PORT/api/notes?q=demo&limit=5"
echo "   GET  /api/state   →"
curl -sS "http://$PORT/api/state"
echo
echo "── page: http://$PORT/   (Ctrl-C, or kill this script)"
wait $PID
