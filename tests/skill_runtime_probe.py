#!/usr/bin/env python3
"""Optional real-runtime discovery probe on macOS, network/file-write sandboxed.

Requires installed Claude Code, Codex and OpenCode. No model invocation or login.
Not a CI dependency: CI uses the deterministic filesystem/recipe E2E tests.
"""
import json
import os
import selectors
import shutil
import subprocess
import time

from test_skill_install import SkillInstallTest


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


def probe():
    sandbox = shutil.which("sandbox-exec")
    if sandbox is None:
        raise RuntimeError("probe requires macOS sandbox-exec to prohibit network and external writes")
    binaries = {}
    for name in ("claude", "codex", "opencode"):
        binary = shutil.which(name)
        if binary is None:
            raise RuntimeError(f"native tool missing: {name}")
        binaries[name] = binary
    fixture = SkillInstallTest()
    fixture.setUp()
    try:
        fixture.apply(fixture.plan())
        cwd = fixture.base / "empty-project"
        cwd.mkdir()
        fixture.command(["git", "init", "-q", str(cwd)])
        home = fixture.home
        (home / ".codex").mkdir()
        # Deliberately do not inherit keys, auth stores, plugin directories, or
        # runtime-specific overrides from the developer's environment.
        env = {"PATH": os.environ["PATH"], "HOME": str(home), "TMPDIR": str(fixture.base),
               "XDG_CONFIG_HOME": str(home / ".config"), "XDG_CACHE_HOME": str(home / ".cache"),
               "XDG_DATA_HOME": str(home / ".local/share"), "XDG_STATE_HOME": str(home / ".local/state"),
               "CODEX_HOME": str(home / ".codex"), "CLAUDE_CONFIG_DIR": str(home / ".claude"),
               "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "DISABLE_AUTOUPDATER": "1",
               "OPENCODE_DISABLE_MODELS_FETCH": "1", "OPENCODE_DISABLE_DEFAULT_PLUGINS": "1",
               "OPENCODE_DISABLE_AUTOUPDATE": "1"}
        policy = '(version 1)(allow default)(deny network*)(deny file-write*)' + \
                 '(allow file-write* (subpath ' + json.dumps(str(fixture.base)) + ') (literal "/dev/null"))'
        prefix = [sandbox, "-p", policy]

        def run(tool, *args):
            result = subprocess.run(prefix + [binaries[tool], *args], cwd=cwd, env=env,
                                    text=True, capture_output=True, timeout=40)
            if result.returncode:
                raise RuntimeError(f"{tool}: {result.stdout}\n{result.stderr}")
            return result.stdout

        versions = {name: run(name, "--version").strip() for name in binaries}
        claude = json.loads(run("claude", "plugin", "validate", str(home / ".claude/skills"), "--json"))
        claude_target = json.loads(run("claude", "plugin", "validate", str(fixture.skill.parent), "--json"))
        if not claude_target.get("success"):
            raise AssertionError(claude_target)
        opencode = json.loads(run("opencode", "debug", "skill", "--pure"))
        # isolate the OpenCode-specific destination: without this, its compatible
        # Claude/Agents scanning could conceal a broken OpenCode symlink.
        fixture.targets[0].unlink()
        fixture.targets[1].unlink()
        opencode_only = json.loads(run("opencode", "debug", "skill", "--pure"))
        fixture.targets[0].symlink_to(fixture.skill)
        fixture.targets[1].symlink_to(fixture.skill)
        log = fixture.base / "codex.stderr"
        with log.open("w") as stderr:
            proc = subprocess.Popen(prefix + [binaries["codex"], "app-server", "--stdio"],
                                    cwd=cwd, env=env, text=True, stdin=subprocess.PIPE,
                                    stdout=subprocess.PIPE, stderr=stderr, bufsize=1)
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
            if len(skills) != 1 or skills[0]["location"] != str(fixture.targets[2] / "SKILL.md"):
                raise AssertionError(f"OpenCode discovery mismatch: {skills}")
            if "result_N_filepath" not in skills[0]["content"]:
                raise AssertionError("OpenCode loaded stale content")
        skills = [entry for group in codex["data"] for entry in group["skills"] if entry["name"] == "backscroll"]
        if len(skills) != 1 or skills[0]["path"] != str(fixture.skill / "SKILL.md") or not skills[0]["enabled"]:
            raise AssertionError(f"Codex discovery mismatch: {skills}")
        print(json.dumps({"versions": versions, "claude_validation": claude, "claude_target_validation": claude_target,
                          "opencode_all_destinations": opencode, "opencode_only": opencode_only,
                          "codex_skills": codex, "codex_stderr": log.read_text()}, indent=2))
    finally:
        fixture.doCleanups()


if __name__ == "__main__":
    probe()
