# Codex `exec_command` Query-Echo Fallback RED

## Question

While a pre-#80 Codex source is still waiting for bounded replay, does the
query-time fallback exclude a `content_type='tool'`, `search_echo=0` row whose
real `SerializeToolInput` text starts with
`exec_command cmd=backscroll search` from both unfiltered result pages and
unfiltered `--relax` document-frequency counting, as it already does for the
equivalent Claude `Bash command=backscroll search` row?

## Hermetic CLI reproduction

The regression test uses the real Codex reader and CLI in a temporary HOME,
config directory, input tree, and SQLite database. It first proves the Claude
control while holding the startup lock so the command must read the existing
snapshot without replay. It then ingests eight Codex calls, sets their
`search_echo` values to zero, holds the same lock, and repeats unfiltered normal
and relaxed searches.

The Codex reader produced this exact serialized input:

```text
exec_command cmd=backscroll search --text 'violet handshake' command=[["unused"]]
```

`SerializeToolInput` sorts object keys, so `cmd` precedes `command` in the real
stored text.

Command:

```console
$ go test ./cmd/backscroll -run TestZeroValuedCodexExecCommandEchoExcludedBeforeReplay -count=1 -v
```

RED output before the production change:

```text
=== RUN   TestZeroValuedCodexExecCommandEchoExcludedBeforeReplay
    echo_zero_replay_e2e_test.go:106: zero-valued Codex fallback leaks=8 want 0: [target.jsonl:text:1 codex-0.jsonl:tool:2 codex-1.jsonl:tool:3 codex-2.jsonl:tool:4 codex-3.jsonl:tool:5 codex-4.jsonl:tool:6 codex-5.jsonl:tool:7 codex-6.jsonl:tool:8 codex-7.jsonl:tool:9]
    echo_zero_replay_e2e_test.go:106: zero-valued query echoes changed unfiltered --relax IDF
        stdout=
--- FAIL: TestZeroValuedCodexExecCommandEchoExcludedBeforeReplay (6.02s)
FAIL
FAIL    github.com/pablontiv/backscroll/cmd/backscroll  6.465s
FAIL
```

The result page contains all eight zero-valued Codex calls while excluding the
zero-valued Claude control. Their document-frequency contribution also makes
relaxation drop a target term rather than `adaptation`, leaving no result.

## Native SQLite predicate probe

The current SQL prefix was also evaluated directly against the two stored-text
shapes:

```console
$ sqlite3 :memory: <<'SQL'
CREATE TABLE search_items(content_type TEXT, search_echo INTEGER, text TEXT);
INSERT INTO search_items VALUES
 ('tool',0,'Bash command=backscroll search --text violet'),
 ('tool',0,'exec_command cmd=backscroll search --text violet command=[["unused"]]');
-- Evaluate the current Bash-prefix GLOB from directBackscrollSearchEchoSQL.
SQL
text                                                          current_sql_echo
------------------------------------------------------------  ----------------
Bash command=backscroll search --text violet                  1
exec_command cmd=backscroll search --text violet command=[["  0
unused"]]
```

## Finding

Confirmed. The Go and SQL query-time fallbacks only encode the Bash prefix;
`unmarkedDirectSearchCallSQL` already encodes the missing `exec_command`
prefix. The smallest fix is to keep the two query-time predicates in lockstep
and add that existing exact prefix, without adding any `shell` argv wrapper
form.
