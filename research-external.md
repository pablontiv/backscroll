I did not write `/home/shared/backscroll/research-external.md` because the task also said “Do not edit files.” Below is the content to place there.

```md
# Research: Declarative ingestion transforms for agent/CLI-agnostic Backscroll

## Summary
External evidence strongly supports designing Backscroll ingestion around declarative input/transformation pipelines rather than hardcoded per-agent parsers. Mature log/data tools expose sources, parsers, filters, mappings, routing, validation, and testability in TOML/YAML-style configs, often with a small expression language for field mapping and filtering.

## Findings

1. **Config-driven pipelines are the dominant ingestion pattern.** Vector, OpenTelemetry Collector, Fluent Bit, Logstash, and Redpanda Connect all define ingestion as configured components connected in order: sources/receivers → parsers/processors/transforms → outputs/exporters. Vector supports YAML/TOML/JSON configs with `sources`, `transforms`, and `sinks`; OpenTelemetry Collector uses YAML `receivers`, `processors`, `exporters`, and `service.pipelines`.  
   Confidence: High.  
   Sources: [Vector configuration](https://vector.dev/docs/reference/configuration/), [OpenTelemetry Collector configuration](https://opentelemetry.io/docs/collector/configuration/)

2. **Declarative parsing avoids hardcoding source semantics.** Fluent Bit parsers transform unstructured logs into structured records and can be reused across inputs; OpenTelemetry filelog receiver chains operators such as `json_parser` and `regex_parser`; Logstash’s JSON filter parses a configured `source` field into root or `target`.  
   Confidence: High.  
   Sources: [Fluent Bit parsers](https://docs.fluentbit.io/manual/data-pipeline/parsers), [OTel filelog receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.148.0/receiver/filelogreceiver/README.md), [Logstash JSON filter](https://www.elastic.co/guide/en/logstash/current/plugins-filters-json.html)

3. **Small mapping DSLs are common for non-trivial reshaping.** Vector’s VRL remap transform is explicitly recommended for parsing, shaping, and transforming observability data; Redpanda Connect’s Bloblang `mapping` processor creates new documents and supports filtering arrays, field projection, fallback behavior, and external mapping files.  
   Confidence: High.  
   Sources: [Vector remap transform](https://vector.dev/docs/reference/configuration/transforms/remap/), [Redpanda Connect mapping](https://docs.redpanda.com/redpanda-connect/components/processors/mapping/)

4. **Filtering and mutation are typically first-class declarative operations.** Fluent Bit’s Modify filter supports add/remove/rename/copy/move rules plus conditions; OTel Collector filter and transform processors use OTTL conditions/statements to drop or mutate telemetry; Logstash applies multiple filters in config order.  
   Confidence: High.  
   Sources: [Fluent Bit Modify filter](https://docs.fluentbit.io/manual/pipeline/filters/modify), [OTel transforming telemetry](https://opentelemetry.io/docs/collector/transforming-telemetry/), [Logstash pipeline structure](https://www.elastic.co/docs/reference/logstash/configuration-file-structure)

5. **Validation, linting, and testability are important companions to declarative ingestion.** Vector provides `validate` and unit tests for configs; OpenTelemetry Collector has `otelcol validate` and `print-config`; Fluent Bit has `--dry-run`; Redpanda Connect has `lint`, `echo`, generated examples, resources, and reload support.  
   Confidence: High.  
   Sources: [Vector validation](https://vector.dev/docs/administration/validating/), [Vector unit tests](https://vector.dev/docs/reference/configuration/unit-tests/), [OTel Collector configuration](https://opentelemetry.io/docs/collector/configuration/), [Redpanda Connect configuration](https://docs.redpanda.com/redpanda-connect/configuration/about/)

## Implications for Backscroll

1. Define an ingestion profile schema in TOML/YAML:
   - `inputs`: path globs, file type, framing such as JSONL/Markdown/plaintext.
   - `decode`: JSON/JSONL, regex, markdown section split, timestamp parsing.
   - `filter`: drop system noise, subagent records, empty messages, etc.
   - `map`: transform raw records into Backscroll canonical fields: `source`, `project`, `session_id`, `role`, `timestamp`, `content`, `metadata`, `tags`.
   - `emit`: allow one input record to produce zero, one, or many searchable items.

2. Prefer a constrained expression/mapping language over arbitrary plugins initially:
   - JSONPath/JMESPath-like selectors for simple extraction.
   - A small safe DSL for conditionals, coalescing, regex capture, type conversion, joining arrays, deleting fields.
   - Optional future plugin hooks for unsupported formats.

3. Treat “Claude Code session JSONL” as one built-in profile, not a special parser path. Other CLIs/agents become additional profiles.

4. Add config validation and dry-run tooling early:
   - `backscroll validate-profile`.
   - `backscroll test-profile --input sample.jsonl`.
   - Show normalized output items and dropped-record reasons.

5. Preserve hardcoded safeguards only where they are product invariants:
   - database schema,
   - canonical search item model,
   - dedup/hash behavior,
   - tokenizer/chunking/indexing mechanics.
   Source-specific semantics should live in profiles.

## Gaps

- I did not find authoritative evidence specifically for AI-agent session indexing tools using declarative ingestion profiles; evidence comes from adjacent mature ingestion systems.
- Expression language choice remains open: adopting an existing language such as JMESPath, CEL, Rhai, Lua, jq-like syntax, or designing a minimal custom DSL requires separate evaluation.
- Security/performance tradeoffs need follow-up, especially if user-defined transforms can execute scripts.

## Sources

- Kept: Vector configuration — demonstrates YAML/TOML/JSON source-transform-sink topology. https://vector.dev/docs/reference/configuration/
- Kept: Vector remap/VRL — strong example of safe, purpose-built transform DSL. https://vector.dev/docs/reference/configuration/transforms/remap/
- Kept: Fluent Bit parsers and Modify filter — reusable parser definitions plus declarative mutation rules. https://docs.fluentbit.io/manual/data-pipeline/parsers
- Kept: Logstash pipeline and JSON filter — mature config-driven plugin/filter architecture. https://www.elastic.co/docs/reference/logstash/configuration-file-structure
- Kept: OpenTelemetry Collector configuration/filelog/transforming telemetry — YAML pipelines, file operators, OTTL filtering/transforms. https://opentelemetry.io/docs/collector/configuration/
- Kept: Redpanda Connect mapping/configuration — YAML pipelines, Bloblang mapping, lint/echo/resources. https://docs.redpanda.com/redpanda-connect/components/processors/mapping/
- Dropped: vendor blog posts and SEO comparisons — less authoritative than official docs.
```