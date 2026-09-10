#!/usr/bin/env python3
"""Filesystem and native-hook E2E regressions; all mutations use a temporary HOME."""
import importlib.util
import json
import os
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
INSTALLER = ROOT / "scripts/install-skills.py"


class SkillInstallTest(unittest.TestCase):
    def setUp(self):
        evidence = ROOT / ".local-evidence"
        evidence.mkdir(exist_ok=True)
        self.tmp = tempfile.TemporaryDirectory(prefix="skill-test-", dir=evidence)
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name).resolve()
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

    def restore(self, receipt, ok=True):
        return self.installer("restore", "--home", str(self.home), "--receipt", receipt["receipt"],
                              "--approve", receipt["restore_approval"], ok=ok)

    def test_plan_is_read_only_and_bound_to_preimage(self):
        self.seed()
        plan = self.plan()
        self.assertFalse((self.home / ".local").exists())
        (self.targets[1] / "local-note").write_text("changed after inventory")
        self.assertIn("approval mismatch", self.apply(plan, ok=False)["error"])
        self.assertFalse((self.home / ".local").exists())
        self.assertFalse(any(t.is_symlink() for t in self.targets))

    def test_source_change_invalidates_approval(self):
        plan = self.plan()
        (self.skill / "SKILL.md").write_text("---\nname: backscroll\ndescription: new\n---\n")
        self.command(["git", "add", "."], cwd=self.source)
        self.commit()
        self.apply(plan, ok=False)
        self.assertFalse(any(t.exists() for t in self.targets))

    def test_source_updates_propagate_without_reinstallation(self):
        self.apply(self.plan())
        with (self.skill / "SKILL.md").open("a") as stream:
            stream.write("\nupdated source sentinel\n")
        self.command(["git", "add", "."], cwd=self.source)
        self.commit()
        for target in self.targets:
            self.assertIn("updated source sentinel", (target / "SKILL.md").read_text())
        self.assertEqual(self.apply(self.plan())["changed"], 0)

    def test_dirty_untracked_ignored_and_linked_sources_rejected(self):
        for name in ("local-overlay", "ignored-overlay"):
            with self.subTest(name=name):
                extra = self.skill / name
                extra.write_text("not product-owned")
                if name.startswith("ignored"):
                    (self.source / ".git/info/exclude").write_text(name + "\n")
                self.installer("plan", "--source-root", str(self.source), "--home", str(self.home), ok=False)
                extra.unlink()
        extra = self.skill / "external"
        extra.symlink_to(self.base)
        self.command(["git", "add", "."], cwd=self.source)
        self.commit()
        self.installer("plan", "--source-root", str(self.source), "--home", str(self.home), ok=False)

    def test_linked_worktree_source_rejected(self):
        linked = self.base / "disposable"
        self.command(["git", "worktree", "add", "--detach", str(linked)], cwd=self.source)
        self.installer("plan", "--source-root", str(linked), "--home", str(self.home), ok=False)

    def test_symlinked_parent_and_source_overlap_rejected(self):
        external = self.base / "outside"
        external.mkdir()
        (self.home / ".agents").symlink_to(external, target_is_directory=True)
        self.installer("plan", "--source-root", str(self.source), "--home", str(self.home), ok=False)
        self.assertEqual(list(external.iterdir()), [])
        self.installer("plan", "--source-root", str(self.source), "--home", str(self.source), ok=False)

    def test_file_and_broken_link_backups_restore_exactly(self):
        for target in self.targets:
            target.parent.mkdir(parents=True)
        self.targets[0].write_bytes(b"file preimage")
        self.targets[0].chmod(0o640)
        self.targets[1].symlink_to("missing-relative-target")
        receipt = self.apply(self.plan())
        self.restore(receipt)
        self.assertEqual(self.targets[0].read_bytes(), b"file preimage")
        self.assertEqual(self.targets[0].stat().st_mode & 0o777, 0o640)
        self.assertEqual(os.readlink(self.targets[1]), "missing-relative-target")
        self.assertFalse(self.targets[2].exists())
        self.assertEqual(self.restore(receipt)["restored"], 0)

    def test_restore_refuses_destination_or_backup_drift(self):
        self.seed()
        receipt = self.apply(self.plan())
        self.targets[0].unlink()
        self.targets[0].write_text("operator's new work")
        self.restore(receipt, ok=False)
        self.assertEqual(self.targets[0].read_text(), "operator's new work")
        self.assertTrue(self.targets[1].is_symlink(), "validate all before restoring any")
        self.targets[0].unlink()
        self.targets[0].symlink_to(self.skill)
        backup = Path(receipt["receipt"]).parent / "backup-1/local-note"
        backup.write_text("backup drift")
        self.restore(receipt, ok=False)
        self.assertTrue(all(t.is_symlink() for t in self.targets))

    def test_partial_failure_rolls_back_all_targets(self):
        self.seed()
        spec = importlib.util.spec_from_file_location("skill_installer", INSTALLER)
        assert spec is not None and spec.loader is not None
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        plan = module.make_plan(self.source, self.home, self.home / ".config", "install")
        original = Path.symlink_to

        def fail_second(path, *args, **kwargs):
            if path == self.targets[1]:
                raise OSError("injected second-destination failure")
            return original(path, *args, **kwargs)

        with mock.patch.object(Path, "symlink_to", fail_second), self.assertRaisesRegex(OSError, "injected"):
            module.apply(self.source, self.home, self.home / ".config", "install", plan["approval"])
        for target in self.targets:
            self.assertFalse(target.is_symlink())
            self.assertEqual((target / "local-note").read_bytes(), b"keep\x00these bytes\n")
        receipt_path = next((self.home / module.STATE).glob("*/receipt.json"))
        receipt = json.loads(receipt_path.read_text())
        self.assertEqual(self.restore({"receipt": str(receipt_path), "restore_approval": module.digest(receipt)})["restored"], 0)

    def test_uninstall_only_owned_links_and_restore(self):
        self.seed()
        self.installer("plan", "--action", "uninstall", "--source-root", str(self.source),
                       "--home", str(self.home), ok=False)
        initial = self.apply(self.plan())
        removed = self.apply(self.plan("uninstall"))
        self.assertFalse(any(t.is_symlink() or t.exists() for t in self.targets))
        self.restore(removed)
        self.assertTrue(all(t.is_symlink() for t in self.targets))
        self.restore(initial)
        self.assertTrue(all((t / "local-note").exists() for t in self.targets))

    def test_receipt_is_append_only_and_restore_token_bound(self):
        receipt = self.apply(self.plan())
        path = Path(receipt["receipt"])
        original = path.read_bytes()
        events = (path.parent / "events.jsonl").read_bytes()
        self.restore({**receipt, "restore_approval": "0" * 64}, ok=False)
        self.restore(receipt)
        self.assertEqual(path.read_bytes(), original)
        self.assertTrue((path.parent / "events.jsonl").read_bytes().startswith(events))

    def test_explicit_config_home_and_duplicate_overlap(self):
        config = self.home / "custom-config"
        plan = self.installer("plan", "--source-root", str(self.source), "--home", str(self.home),
                              "--config-home", str(config))
        self.installer("apply", "--source-root", str(self.source), "--home", str(self.home),
                       "--config-home", str(config), "--approve", plan["approval"])
        self.assertTrue((config / "opencode/skills/backscroll").is_symlink())
        self.assertFalse(self.targets[2].exists())
        self.installer("plan", "--source-root", str(self.source), "--home", str(self.home),
                       "--config-home", str(self.targets[0]), ok=False)

    @unittest.skipUnless(os.environ.get("BACKSCROLL_TEST_BINARY"), "set BACKSCROLL_TEST_BINARY to a dev build")
    def test_installed_recipe_retrieves_known_fixture(self):
        binary = Path(os.environ["BACKSCROLL_TEST_BINARY"]).resolve()
        self.assertIn("dev", self.command([str(binary), "--version"]).stdout,
                      "release identities may autoupdate; use a dev build")
        self.apply(self.plan())
        cwd = self.base / "application"
        cwd.mkdir()
        sessions = self.base / "sessions"
        sessions.mkdir()
        document = sessions / "fixture.jsonl"
        document.write_text(json.dumps({"type": "user", "uuid": "skill-install-fixture", "cwd": str(cwd),
                            "timestamp": "2026-01-01T00:00:00Z", "message": {"role": "user",
                            "content": "installationoraclecobalt"}}) + "\n")
        config = self.home / ".config/backscroll"
        config.mkdir(parents=True, exist_ok=True)
        (config / "projects.toml").write_text('[[projects]]\nid = "skill-fixture"\nroots = [' + json.dumps(str(cwd)) + ']\n')
        self.env["BACKSCROLL_SESSION_DIRS"] = str(sessions)
        self.env["BACKSCROLL_DATABASE_PATH"] = str(self.base / "index.db")
        for target in self.targets:
            with self.subTest(target=target):
                recipe = (target / "SKILL.md").read_text()
                line = next(line for line in recipe.splitlines() if line.startswith('backscroll search "QUERY" --robot'))
                argv = shlex.split(line.replace('"QUERY"', '"installationoraclecobalt"'))
                result = self.command([str(binary), *argv[1:]], cwd=cwd)
                fields = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
                self.assertEqual(fields.get("result_0_filepath"), str(document), result.stdout + result.stderr)
                self.assertNotIn("result_0_source_path", fields)

    def test_skip_worktree_does_not_hide_uncommitted_source(self):
        self.command(["git", "update-index", "--skip-worktree", ".claude/skills/backscroll/SKILL.md"], cwd=self.source)
        (self.skill / "SKILL.md").write_text("hidden uncommitted edit")
        self.installer("plan", "--source-root", str(self.source), "--home", str(self.home), ok=False)

    def test_unsafe_state_and_special_preimages_rejected(self):
        self.seed()
        plan = self.plan()
        state = self.home / ".local/state/backscroll/skill-install"
        state.parent.mkdir(parents=True)
        outside = self.base / "outside"
        outside.mkdir()
        state.symlink_to(outside)
        self.apply(plan, ok=False)
        self.assertEqual(list(outside.iterdir()), [])
        self.assertTrue(all((t / "local-note").exists() for t in self.targets))
        state.unlink()
        os.mkfifo(self.targets[0] / "pipe")
        self.installer("plan", "--source-root", str(self.source), "--home", str(self.home), ok=False)

    def test_restore_after_process_exit_during_second_destination(self):
        self.seed()
        # Kill only this fixture subprocess after the second preimage rename.
        code = '''import importlib.util, os, sys
from pathlib import Path
spec = importlib.util.spec_from_file_location("installer", sys.argv[1])
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
root, home = Path(sys.argv[2]), Path(sys.argv[3])
original = Path.symlink_to
def stop(path, *args, **kwargs):
    if ".agents" in path.parts:
        os._exit(71)
    return original(path, *args, **kwargs)
Path.symlink_to = stop
p = m.make_plan(root, home, home / ".config", "install")
m.apply(root, home, home / ".config", "install", p["approval"])
'''
        result = subprocess.run([sys.executable, "-c", code, str(INSTALLER), str(self.source), str(self.home)],
                                env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 71, result.stderr)
        self.assertTrue(self.targets[0].is_symlink())
        self.assertFalse(self.targets[1].exists())
        receipt_path = next((self.home / ".local/state/backscroll/skill-install").glob("*/receipt.json"))
        event = json.loads((receipt_path.parent / "events.jsonl").read_text().splitlines()[0])
        self.restore({"receipt": str(receipt_path), "restore_approval": event["restore_approval"]})
        self.assertTrue(all((t / "local-note").exists() for t in self.targets))

    def test_installed_recipe_has_observed_robot_keys(self):
        self.apply(self.plan())
        for target in self.targets:
            text = (target / "SKILL.md").read_text()
            self.assertIn("result_N_filepath", text)
            self.assertNotIn("result_N_source_path", text)
            self.assertNotIn("--project <cwd-or-inferred>", text)


if __name__ == "__main__":
    unittest.main()
