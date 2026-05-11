#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)

assert_contains() {
  file=$1
  expected=$2

  if ! grep -Fq "$expected" "$file"; then
    echo "expected ${file#$repo_root/} to contain: $expected" >&2
    cat "$file" >&2
    exit 1
  fi
}

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

events="$tmpdir/go-test-events.json"
summary="$tmpdir/go-test-timing.md"

cat >"$events" <<'JSON'
{"Time":"2026-05-11T00:00:00Z","Action":"run","Package":"github.com/zack-nova/harnessyard/pkg/fast","Test":"TestFast"}
{"Time":"2026-05-11T00:00:00Z","Action":"pass","Package":"github.com/zack-nova/harnessyard/pkg/fast","Test":"TestFast","Elapsed":0.02}
{"Time":"2026-05-11T00:00:00Z","Action":"pass","Package":"github.com/zack-nova/harnessyard/pkg/fast","Elapsed":0.04}
{"Time":"2026-05-11T00:00:00Z","Action":"run","Package":"github.com/zack-nova/harnessyard/pkg/slow","Test":"TestSlow"}
{"Time":"2026-05-11T00:00:00Z","Action":"pass","Package":"github.com/zack-nova/harnessyard/pkg/slow","Test":"TestSlow","Elapsed":1.27}
{"Time":"2026-05-11T00:00:00Z","Action":"skip","Package":"github.com/zack-nova/harnessyard/pkg/slow","Test":"TestSkipped","Elapsed":0.15}
{"Time":"2026-05-11T00:00:00Z","Action":"pass","Package":"github.com/zack-nova/harnessyard/pkg/slow","Elapsed":1.35}
JSON

cd "$repo_root"

go run ./internal/citest/gotesttiming \
  -input "$events" \
  -shard "fixture-shard" \
  -limit 2 >"$summary"

assert_contains "$summary" "## Go test timing: fixture-shard"
assert_contains "$summary" "| github.com/zack-nova/harnessyard/pkg/slow | pass | 1.350s |"
assert_contains "$summary" "| github.com/zack-nova/harnessyard/pkg/fast | pass | 0.040s |"
assert_contains "$summary" "| github.com/zack-nova/harnessyard/pkg/slow | TestSlow | pass | 1.270s |"
assert_contains "$summary" "| github.com/zack-nova/harnessyard/pkg/slow | TestSkipped | skip | 0.150s |"
assert_contains "$summary" "go-test-events-fixture-shard.json"

real_go=$(command -v go)
fake_bin="$tmpdir/fake-bin"
runner_output="$tmpdir/runner-output.txt"
mkdir -p "$fake_bin"

cat >"$fake_bin/go" <<EOF_FAKE_GO
#!/bin/sh

case "\$1" in
  test)
    printf '%s\n' '{"Action":"fail","Package":"github.com/zack-nova/harnessyard/failing","Elapsed":0.17}'
    exit 7
    ;;
  run)
    exec "$real_go" "\$@"
    ;;
  *)
    exec "$real_go" "\$@"
    ;;
esac
EOF_FAKE_GO
chmod +x "$fake_bin/go"

set +e
PATH="$fake_bin:$PATH" \
  GO_TEST_TIMING_DIR="$tmpdir/runner-timing" \
  GO_TEST_TIMING_LIMIT=1 \
  sh ./scripts/run_go_test_shard.sh hyard-cli >"$runner_output" 2>&1
runner_status=$?
set -e

if [ "$runner_status" -ne 7 ]; then
  echo "expected run_go_test_shard.sh to preserve go test status 7, got $runner_status" >&2
  cat "$runner_output" >&2
  exit 1
fi
assert_contains "$runner_output" "## Go test timing: hyard-cli"
assert_contains "$runner_output" "| github.com/zack-nova/harnessyard/failing | fail | 0.170s |"

echo "Go test timing summary tests passed"
