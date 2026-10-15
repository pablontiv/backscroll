#!/usr/bin/env python3
"""Optional macOS discovery probe for the native Backscroll skill installer.

Pass a dev Backscroll binary as the first argument. The probe uses a temporary
HOME. It blocks network access and blocks writes outside the temporary fixture.
"""
import json
import os
from pathlib import Path
import selectors
import shutil
import subprocess
import sys
import tempfile
import time


def rpc(proc, message, expected):
    assert proc.stdin is not None and proc.stdout is not None
    proc.stdin.write(json.dumps(message) + "\n")
    proc.stdin.flush()
    deadline = time.monotonic() + 30
    with selectors.DefaultSelector() as selector:
        selector.register(proc.stdout, selectors.EVENT_READ)
        while time.monotonic() < deadline:
            if not selector.select(timeout=max(0, deadline - time.monotonic())):
                break
            line = proc.stdout.readline()
            if not line:
                break
            response = json.loads(line)
            if response.get("id") == expected:
                if "error" in response:
                    raise RuntimeError(response)
                return response["result"]
    raise RuntimeError(f"no response to {message['method']}")


def probe(binary):
    sandbox = shutil.which("sandbox-exec")
    if sandbox is None:
        raise RuntimeError("probe requires macOS sandbox-exec")
    runtimes = {}
    for name in ("claude", "codex", "opencode"):
        path = shutil.which(name)
        if path is None:
            raise RuntimeError(f"native tool missing: {name}")
        runtimes[name] = path

    evidence = Path(__file__).resolve().parents[1] / ".local-evidence"
    evidence.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="skill-runtime-", dir=evidence) as temporary:
        base = Path(temporary).resolve()
        home = base / "home"
        config_home = home / ".config"
        cwd = base / "empty-project"
        home.mkdir()
        cwd.mkdir()
        subprocess.run(["git", "init", "-q", str(cwd)], check=True)
        env = {
            "PATH": os.environ["PATH"],
            "HOME": str(home),
            "TMPDIR": str(base),
            "XDG_CONFIG_HOME": str(config_home),
            "XDG_CACHE_HOME": str(home / ".cache"),
            "XDG_DATA_HOME": str(home / ".local/share"),
            "XDG_STATE_HOME": str(home / ".local/state"),
            "CODEX_HOME": str(home / ".codex"),
            "CLAUDE_CONFIG_DIR": str(home / ".claude"),
            "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
            "DISABLE_AUTOUPDATER": "1",
            "OPENCODE_DISABLE_MODELS_FETCH": "1",
            "OPENCODE_DISABLE_DEFAULT_PLUGINS": "1",
            "OPENCODE_DISABLE_AUTOUPDATE": "1",
        }
        (home / ".codex").mkdir()
        installed = subprocess.run(
            [str(binary), "skill", "--install", "--home", str(home),
             "--config-home", str(config_home)],
            env=env, cwd=cwd, text=True, capture_output=True, check=True,
        )
        install_result = json.loads(installed.stdout)
        targets = [
            home / ".claude/skills/backscroll",
            home / ".agents/skills/backscroll",
            config_home / "opencode/skills/backscroll",
        ]
        managed = targets[0].resolve(strict=True)
        if any(target.resolve(strict=True) != managed for target in targets):
            raise AssertionError("installed destinations do not share one managed shim")

        policy = (
            '(version 1)(allow default)(deny network*)(deny file-write*)'
            f'(allow file-write* (subpath {json.dumps(str(base))}) (literal "/dev/null"))'
        )
        prefix = [sandbox, "-p", policy]

        def run(tool, *args):
            result = subprocess.run(
                prefix + [runtimes[tool], *args], cwd=cwd, env=env,
                text=True, capture_output=True, timeout=40,
            )
            if result.returncode:
                raise RuntimeError(f"{tool}: {result.stdout}\n{result.stderr}")
            return result.stdout

        versions = {name: run(name, "--version").strip() for name in runtimes}
        claude = json.loads(run("claude", "plugin", "validate", str(home / ".claude/skills"), "--json"))
        claude_managed = json.loads(run("claude", "plugin", "validate", str(managed.parent), "--json"))
        if not claude_managed.get("success"):
            raise AssertionError(claude_managed)
        opencode = json.loads(run("opencode", "debug", "skill", "--pure"))
        targets[0].unlink()
        targets[1].unlink()
        opencode_only = json.loads(run("opencode", "debug", "skill", "--pure"))
        targets[0].symlink_to(managed)
        targets[1].symlink_to(managed)

        log = base / "codex.stderr"
        with log.open("w") as stderr:
            proc = subprocess.Popen(
                prefix + [runtimes["codex"], "app-server", "--stdio"],
                cwd=cwd, env=env, text=True, stdin=subprocess.PIPE,
                stdout=subprocess.PIPE, stderr=stderr, bufsize=1,
            )
            try:
                rpc(proc, {"id": 1, "method": "initialize", "params": {
                    "clientInfo": {"name": "backscroll-discovery", "version": "1"}}}, 1)
                assert proc.stdin is not None
                proc.stdin.write(json.dumps({"method": "initialized", "params": {}}) + "\n")
                proc.stdin.flush()
                codex = rpc(proc, {"id": 2, "method": "skills/list", "params": {
                    "cwds": [str(cwd)], "forceReload": True}}, 2)
            finally:
                proc.terminate()
                proc.wait(timeout=10)
                if proc.stdin:
                    proc.stdin.close()
                if proc.stdout:
                    proc.stdout.close()

        for result in (opencode, opencode_only):
            skills = [entry for entry in result if entry.get("name") == "backscroll"]
            if len(skills) != 1 or skills[0]["location"] != str(targets[2] / "SKILL.md"):
                raise AssertionError(f"OpenCode discovery mismatch: {skills}")
            if "backscroll --skill" not in skills[0]["content"]:
                raise AssertionError("OpenCode did not load the discovery shim")
        skills = [entry for group in codex["data"] for entry in group["skills"]
                  if entry["name"] == "backscroll"]
        if len(skills) != 1 or Path(skills[0]["path"]).resolve() != managed / "SKILL.md" or not skills[0]["enabled"]:
            raise AssertionError(f"Codex discovery mismatch: {skills}")
        print(json.dumps({
            "install": install_result,
            "versions": versions,
            "claude_validation": claude,
            "claude_managed_validation": claude_managed,
            "opencode_all_destinations": opencode,
            "opencode_only": opencode_only,
            "codex_skills": codex,
            "codex_stderr": log.read_text(),
        }, indent=2))


if __name__ == "__main__":
    selected = Path(sys.argv[1] if len(sys.argv) > 1 else shutil.which("backscroll") or "").resolve()
    if not selected.is_file():
        raise SystemExit("pass a dev Backscroll binary as the first argument")
    probe(selected)
