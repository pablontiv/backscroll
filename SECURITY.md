# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Backscroll, please report it responsibly.

**Do not open a public issue.** Instead, email the maintainer directly or use [GitHub's private vulnerability reporting](https://github.com/pablontiv/backscroll/security/advisories/new).

Include:

- Description of the vulnerability
- Steps to reproduce
- Potential impact

You will receive acknowledgment within 48 hours and a detailed response within 7 days.

## Scope

Backscroll is a CLI tool that reads local JSONL files and indexes them into SQLite. Security concerns may include:

- SQLite injection via crafted JSONL content
- Path traversal in `--path` or `read` arguments
- Denial of service via malformed session files
- Dependencies with known CVEs
