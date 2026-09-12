# Echo-Exclusion Chokepoint Spike

Date: 2026-09-12
Branch: `fm/bs-echo-chokepoint-hardening-r1`
Status: decided — proceed to implementation

## Question

Can the three independent recognizers of "is this stored row a direct
`backscroll search` echo" (page-exclusion Go predicate in
`internal/storage/search.go`, IDF-counting SQL GLOB in
`directBackscrollSearchEchoSQL`, requeue SQL GLOB in
`unmarkedDirectSearchCallSQL` + shell-only Go fallback) be replaced by one
strict Go predicate behind one broad SQL prefilter, without changing any
accepted boundary case — and does that structurally close the separator
alphabet divergence class (#64/#86/#87/#89) instead of point-patching the SQL
whitespace list?

## PoC (RED)

`internal/storage/echo_parity_test.go` generates the cross product of
serialized shape {bash, exec_command, shell} × separator alphabet {ASCII
controls, NBSP, U+2028, U+2029, U+3000, repeated/mixed runs} ×
leading/trailing runs, stores each fixture as a zero-valued (`search_echo=0`)
tool row, and asserts three-way agreement: unfiltered page exclusion
(`isDirectBackscrollSearchEcho`) == unfiltered `--relax` IDF exclusion
(`recallFrequency`) == requeue detection (`PendingSearchEchoPaths`).

Against the pre-change code it fails 48 subtests, all of them the bug class,
none a boundary dispute:

- `bash`/`exec_command` × {NBSP, U+2028, U+2029, U+3000, mixed runs}: the Go
  page predicate excludes the row (`strings.Fields` accepts every
  `unicode.IsSpace` rune); the SQL GLOB separator alphabet is ASCII-only, so
  IDF counting and requeue keep it. Patching NBSP into the GLOB class would
  leave U+2028/U+2029/U+3000 and every future rune — confirming the point
  patch is structurally insufficient.
- `shell` × leading-run: the `text LIKE 'shell %'` prefilter requires the row
  to start with `shell`, so a leading whitespace run makes IDF/requeue miss
  a row the Go decode excludes. A substring prefilter (`%backscroll%`) has no
  anchor and no alphabet.

All negative controls (status/searcher subcommands, absolute paths, env
wrappers, folded key case, nested `bash -lc`, prose mentions) already pass
and must keep passing unchanged.

## Verdict

Proceed. Every accepted echo row contains the literal lowercase substring
`backscroll` in its stored text (bash: `command=backscroll`; exec_command:
`cmd=backscroll`; shell: the JSON-encoded argv), so
`content_type='tool' AND COALESCE(search_echo,0)=0 AND text LIKE '%backscroll%'`
is a provable superset prefilter; the strict
`directsearch.IsSerializedDirectSearchCall` predicate applied in Go on the
prefiltered rows is then the single definition of the boundary for pages,
IDF, and requeue alike. No schema migration; deletes more code than it adds.
