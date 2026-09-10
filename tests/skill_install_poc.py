#!/usr/bin/env python3
"""Native distribution probe, isolated HOME; no push, network or installed binary.

Build a dev backscroll binary first and pass it as the sole argument. Hook build
and rootline are stubbed: only the unchanged native skill-distribution path is
under observation. The retrieval command uses the real dev binary.
"""
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def run(argv, **kwargs):
    return subprocess.run(argv, check=True, text=True, capture_output=True, **kwargs)


def probe(binary):
    with tempfile.TemporaryDirectory(prefix="skill-poc-", dir=ROOT / ".local-evidence") as tmp:
        base = Path(tmp)
        home = base / "home"
        home.mkdir()
        config = home / ".config"
        stub = base / "bin"
        stub.mkdir()
        # Do not rebuild/install a release-identity binary or run rootline fix.
        for name in ("go", "rootline"):
            path = stub / name
            path.write_text("#!/bin/sh\nexit 1\n")
            path.chmod(0o755)
        env = {**os.environ, "HOME": str(home), "XDG_CONFIG_HOME": str(config),
               "XDG_DATA_HOME": str(home / ".local/share"), "XDG_CACHE_HOME": str(home / ".cache"),
               "BACKSCROLL_CONFIG_DIR": str(config), "BACKSCROLL_DATABASE_PATH": str(base / "index.db"),
               "BACKSCROLL_BIN": str(base / "not-installed"), "TMPDIR": str(base),
               "PATH": str(stub) + os.pathsep + os.environ["PATH"]}
        destinations = [home / ".claude/skills/backscroll", home / ".agents/skills/backscroll",
                        config / "opencode/skills/backscroll"]
        for dest in destinations:
            dest.mkdir(parents=True)
            (dest / "SKILL.md").write_text("STALE --project <cwd-or-inferred>\n")
            (dest / "local-note").write_text("preserve me\n")
        native = subprocess.run(["bash", str(ROOT / ".githooks/pre-push")], input="", env=env,
                                cwd=ROOT, text=True, capture_output=True)
        observed = [{"target": str(d.relative_to(home)), "link": d.is_symlink(),
                     "sentinel_retained": (d / "local-note").exists(),
                     "stale": "STALE" in (d / "SKILL.md").read_text()} for d in destinations]
        # Isolated declarative input; no raw session inspection or real index.
        inputs = config / "backscroll/inputs"
        inputs.mkdir(parents=True, exist_ok=True)
        for preset in inputs.glob("*.inputs.toml"):
            preset.unlink()
        fixture = base / "fixture"
        fixture.mkdir()
        document = fixture / "recall.md"
        document.write_text("# Recall\n\ninstallationoraclecobalt\n")
        (inputs / "fixture.inputs.toml").write_text(
            'version = 1\n[[inputs]]\nid = "fixture"\nsource = "memory"\nactive = true\n'
            '[inputs.discover]\nroots = [' + json.dumps(str(fixture)) + ']\ninclude = ["**/*.md"]\n'
            '[inputs.decode]\nformat = "markdown_document"\n')
        search = run([str(binary), "search", "installationoraclecobalt", "--all-projects",
                      "--robot", "--fields", "minimal", "--max-tokens", "2000"], env=env, cwd=base)
        fields = dict(line.split("=", 1) for line in search.stdout.splitlines() if "=" in line)
        assert fields.get("result_0_filepath") == str(document), search.stdout + search.stderr
        assert "result_0_source_path" not in fields
        print(json.dumps({"native_hook_exit": native.returncode, "targets": observed,
                          "robot": search.stdout, "search_stderr": search.stderr}, indent=2))


if __name__ == "__main__":
    probe(Path(sys.argv[1]).resolve())
