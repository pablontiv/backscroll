#!/usr/bin/env python3
"""Filesystem and native-hook E2E regressions; all mutations use a temporary HOME."""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INSTALLER = ROOT / "scripts/install-skills.py"


class SkillInstallTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="skill-test-")
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = self.base / "home"
        self.home.mkdir()
        self.source = self.base / "stable-product"
        self.env = {**os.environ, "HOME": str(self.home), "XDG_CONFIG_HOME": str(self.home / ".config"),
                    "XDG_DATA_HOME": str(self.home / ".local/share"),
                    "XDG_CACHE_HOME": str(self.home / ".cache"), "TMPDIR": str(self.base),
                    "BACKSCROLL_CONFIG_DIR": str(self.home / ".config"),
                    "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"}
        for key in list(self.env):
            if key.startswith("GIT_") and key not in ("GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"):
                del self.env[key]
        self.command(["git", "init", "-q", str(self.source)])
        self.skill = self.source / ".claude/skills/backscroll"
        shutil.copytree(ROOT / ".claude/skills/backscroll", self.skill)
        self.command(["git", "add", "."], cwd=self.source)
        self.commit()
        self.targets = [self.home / ".claude/skills/backscroll", self.home / ".agents/skills/backscroll",
                        self.home / ".config/opencode/skills/backscroll"]

    def command(self, argv, **kwargs):
        return subprocess.run(argv, env=self.env, text=True, capture_output=True, check=True, **kwargs)

    def commit(self):
        self.command(["git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                      "-c", "core.hooksPath=/dev/null", "commit", "-qm", "fixture"], cwd=self.source)

    def installer(self, *args, ok=True):
        result = subprocess.run([sys.executable, str(INSTALLER), *args], env=self.env,
                                text=True, capture_output=True)
        if ok:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            return json.loads(result.stdout)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return {"error": result.stderr}

    def plan(self, action="install"):
        return self.installer("plan", "--source-root", str(self.source), "--home", str(self.home),
                              "--action", action)

    def apply(self, plan, ok=True):
        return self.installer("apply", "--source-root", str(self.source), "--home", str(self.home),
                              "--action", plan["action"], "--approve", plan["approval"], ok=ok)

    def seed(self):
        for target in self.targets:
            target.mkdir(parents=True)
            (target / "SKILL.md").write_text("STALE --project <cwd-or-inferred>\n")
            (target / "local-note").write_bytes(b"keep\x00these bytes\n")

    def test_native_hooks_preserve_existing_skills(self):
        self.seed()
        stub = self.base / "bin"
        stub.mkdir()
        for name in ("go", "rootline"):
            (stub / name).write_text("#!/bin/sh\nexit 1\n")
            (stub / name).chmod(0o755)
        self.env["PATH"] = str(stub) + os.pathsep + os.environ["PATH"]
        self.env["BACKSCROLL_BIN"] = str(self.base / "uninstalled-binary")
        for hook in ("pre-push", "post-merge"):
            self.command(["bash", str(ROOT / ".githooks" / hook)], cwd=ROOT, input="")
            for target in self.targets:
                self.assertTrue((target / "local-note").exists(), f"{hook} destroyed {target}")
                self.assertIn("STALE", (target / "SKILL.md").read_text())

    def test_install_all_targets_backup_repeat_and_restore(self):
        self.seed()
        plan = self.plan()
        for target in self.targets:
            self.assertFalse(target.is_symlink(), "plan must be read-only")
        receipt = self.apply(plan)
        for target in self.targets:
            self.assertTrue(target.is_symlink())
            self.assertEqual(os.readlink(target), str(self.skill))
            self.assertEqual(target.resolve(strict=True), self.skill)
            self.assertEqual((target / "SKILL.md").read_bytes(), (self.skill / "SKILL.md").read_bytes())
        self.assertEqual(self.apply(self.plan())["changed"], 0)
        restored = self.installer("restore", "--home", str(self.home), "--receipt", receipt["receipt"],
                                  "--approve", receipt["restore_approval"])
        self.assertEqual(restored["restored"], 3)
        for target in self.targets:
            self.assertFalse(target.is_symlink())
            self.assertEqual((target / "local-note").read_bytes(), b"keep\x00these bytes\n")


if __name__ == "__main__":
    unittest.main()
