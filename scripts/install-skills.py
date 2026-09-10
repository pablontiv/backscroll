#!/usr/bin/env python3
"""Explicit POSIX Backscroll skill installation. See docs/skill-installation.md.

plan is read-only; apply requires the exact plan digest. All replaced objects are
renamed into a private transaction directory. Receipts precede mutation and a
restore operation is safe to repeat after an interrupted install or restore.
Requires Python 3.9+, Git, and a stable ordinary clone (not a linked worktree).
"""

import argparse
import contextlib
import fcntl
import hashlib
import json
import os
import stat
import subprocess
import sys
import uuid
from pathlib import Path

SKILL = Path(".claude/skills/backscroll")
STATE = Path(".local/state/backscroll/skill-install")


def digest(value):
    return hashlib.sha256(
        json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    ).hexdigest()


def safe_parents(path):
    """Never follow a symlinked destination/receipt ancestor."""
    for parent in reversed(path.parents):
        if parent.is_symlink() or (parent.exists() and not parent.is_dir()):
            raise ValueError(f"unsafe ancestor: {parent}")


def absolute(path):
    result = Path(os.path.abspath(path))
    safe_parents(result)
    return result


def snapshot(path):
    """Exact content, mode and lexical links; never traverse links or special files."""
    try:
        meta = path.lstat()
    except FileNotFoundError:
        return {"kind": "absent"}
    mode = stat.S_IMODE(meta.st_mode)
    if stat.S_ISLNK(meta.st_mode):
        return {"kind": "link", "target": os.readlink(path)}
    if stat.S_ISREG(meta.st_mode):
        if meta.st_nlink != 1:
            raise ValueError(f"hard-linked file not supported: {path}")
        return {
            "kind": "file",
            "mode": mode,
            "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
        }
    if stat.S_ISDIR(meta.st_mode):
        return {
            "kind": "directory",
            "mode": mode,
            "entries": {
                child.name: snapshot(child) for child in sorted(path.iterdir())
            },
        }
    raise ValueError(f"special file not supported: {path}")


def source_info(root):
    root = absolute(root)
    if (
        root.is_symlink()
        or not (root / ".git").is_dir()
        or (root / ".git").is_symlink()
    ):
        raise ValueError(
            "source-root must be a stable ordinary clone, not a linked worktree"
        )
    env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
    env["GIT_OPTIONAL_LOCKS"] = "0"  # inventory must not refresh/write the source index
    command = ["git", "-c", "core.fsmonitor=false", "-C", str(root)]

    def git(*args):
        return subprocess.check_output([*command, *args], env=env, text=True).strip()

    if Path(git("rev-parse", "--show-toplevel")) != root:
        raise ValueError("source-root must name the clone root")
    source = root / SKILL
    safe_parents(source)
    tree = snapshot(source)
    if tree["kind"] != "directory" or not (source / "SKILL.md").is_file():
        raise ValueError("canonical SKILL.md missing")

    # Keep distribution entirely product-owned; reject local overlays/links.
    def check_tree(node):
        if node["kind"] == "link":
            raise ValueError("canonical skill tree must not contain symlinks")
        for child in node.get("entries", {}).values():
            check_tree(child)

    check_tree(tree)
    if git(
        "status", "--porcelain", "--untracked-files=all", "--ignored", "--", str(SKILL)
    ):
        raise ValueError("canonical skill must be clean and fully tracked")
    git("ls-files", "--error-unmatch", str(SKILL / "SKILL.md"))
    commit = git("rev-parse", "HEAD")
    # status alone can miss assume-unchanged / skip-worktree edits. Compare
    # committed blob bytes too; never ship a local overlay as a committed source.
    records = subprocess.check_output(
        [*command, "ls-tree", "-rz", commit, "--", str(SKILL)], env=env
    )
    for record in records.split(b"\0"):
        if not record:
            continue
        header, name = record.split(b"\t", 1)
        mode, kind, oid = header.split()
        if kind != b"blob" or mode not in (b"100644", b"100755"):
            raise ValueError("canonical source contains non-file Git entries")
        content = subprocess.check_output(
            [*command, "cat-file", "blob", oid.decode()], env=env
        )
        if (root / os.fsdecode(name)).read_bytes() != content:
            raise ValueError("canonical skill bytes differ from the source commit")
    return {
        "root": str(root),
        "path": str(source),
        "commit": commit,
        "tree_sha256": digest(tree),
    }


def targets(home, config_home):
    return [
        home / ".claude/skills/backscroll",
        home / ".agents/skills/backscroll",
        config_home / "opencode/skills/backscroll",
    ]


def make_plan(root, home, config_home, action):
    source = source_info(root)
    destinations = targets(home, config_home)
    protected = [Path(source["path"]), home / STATE]
    for i, target in enumerate(destinations):
        for other in protected + destinations[i + 1 :]:
            if target == other or target in other.parents or other in target.parents:
                raise ValueError(
                    f"overlapping source, state or destinations: {target}, {other}"
                )
    if (
        home / STATE == Path(source["root"])
        or home / STATE in Path(source["root"]).parents
    ):
        raise ValueError("source must not live inside installation state")
    rows = []
    for target in destinations:
        safe_parents(target)
        before = snapshot(target)
        linked = before == {"kind": "link", "target": source["path"]}
        if action == "uninstall" and before["kind"] != "absent" and not linked:
            raise ValueError(f"uninstall refuses an unowned destination: {target}")
        rows.append(
            {
                "path": str(target),
                "before": before,
                "change": not linked if action == "install" else linked,
            }
        )
    plan = {
        "version": 1,
        "action": action,
        "home": str(home),
        "config_home": str(config_home),
        "source": source,
        "targets": rows,
    }
    return {**plan, "approval": digest(plan)}


def private_dir(path):
    safe_parents(path)
    if path.is_symlink():
        raise ValueError(f"unsafe state directory: {path}")
    path.mkdir(parents=True, exist_ok=True, mode=0o700)
    if path.stat().st_uid != os.getuid() or stat.S_IMODE(path.stat().st_mode) != 0o700:
        raise ValueError(f"state directory must be owned by you and mode 0700: {path}")


@contextlib.contextmanager
def locked(home):
    state = home / STATE
    private_dir(state)
    fd = os.open(state / "lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        if os.fstat(fd).st_nlink != 1:
            raise ValueError("unsafe lock file")
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield state
    finally:
        os.close(fd)


def append_event(run, event):
    fd = os.open(
        run / "events.jsonl",
        os.O_WRONLY | os.O_APPEND | os.O_CREAT | os.O_NOFOLLOW,
        0o600,
    )
    with os.fdopen(fd, "a") as stream:
        if os.fstat(stream.fileno()).st_nlink != 1:
            raise ValueError("unsafe event file")
        stream.write(json.dumps(event, sort_keys=True) + "\n")
        stream.flush()
        os.fsync(stream.fileno())


def sync_dir(path):
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def restore_rows(run, receipt):
    """Validate every preimage first. Preserve all evidence if anything drifted."""
    plan = receipt["plan"]
    after = (
        {"kind": "link", "target": plan["source"]["path"]}
        if plan["action"] == "install"
        else {"kind": "absent"}
    )
    pending = []
    for i, row in enumerate(plan["targets"]):
        if not row["change"]:
            continue
        target = Path(row["path"])
        safe_parents(target)
        current = snapshot(target)
        backup = run / f"backup-{i}"
        saved = snapshot(backup)
        before = row["before"]
        if current == before and saved["kind"] == "absent":
            continue  # not started, or already restored
        if current not in (after, {"kind": "absent"}) or saved != before:
            raise ValueError(f"restore refuses changed destination or backup: {target}")
        pending.append((target, backup, before))
    for target, backup, before in pending:
        append_event(run, {"event": "restore-intent", "path": str(target)})
        if target.is_symlink():
            target.unlink()
        if before["kind"] != "absent":
            backup.rename(target)
        sync_dir(target.parent)
        sync_dir(run)
        append_event(run, {"event": "restored", "path": str(target)})
    return len(pending)


def apply(root, home, config_home, action, approval):
    # Check approval before even creating operational state.
    plan = make_plan(root, home, config_home, action)
    if approval != plan["approval"]:
        raise ValueError("approval mismatch; inspect a fresh plan before authorizing")
    with locked(home) as state:
        if make_plan(root, home, config_home, action) != plan:
            raise ValueError("inventory changed before installation")
        run = state / uuid.uuid4().hex
        private_dir(run)
        receipt = {"version": 1, "plan": plan, "run": run.name}
        restore_approval = digest(receipt)
        with (run / "receipt.json").open("x") as stream:
            os.chmod(run / "receipt.json", 0o600)
            json.dump(receipt, stream, indent=2)
            stream.flush()
            os.fsync(stream.fileno())
        sync_dir(run)
        sync_dir(state)
        append_event(run, {"event": "prepared", "restore_approval": restore_approval})
        changed = 0
        try:
            for i, row in enumerate(plan["targets"]):
                if not row["change"]:
                    continue
                target = Path(row["path"])
                safe_parents(target)
                if snapshot(target) != row["before"]:
                    raise ValueError(
                        f"destination changed during installation: {target}"
                    )
                target.parent.mkdir(parents=True, exist_ok=True)
                if target.parent.stat().st_dev != run.stat().st_dev:
                    raise ValueError(
                        "backup and destination must be on the same filesystem"
                    )
                append_event(run, {"event": "install-intent", "path": str(target)})
                if row["before"]["kind"] != "absent":
                    target.rename(run / f"backup-{i}")
                    sync_dir(run)
                    sync_dir(target.parent)
                if action == "install":
                    target.symlink_to(plan["source"]["path"], target_is_directory=True)
                sync_dir(target.parent)
                changed += 1
                append_event(run, {"event": "installed", "path": str(target)})
            if source_info(root) != plan["source"]:
                raise ValueError("canonical source changed during installation")
        except Exception:
            print(
                f"recovery receipt: {run / 'receipt.json'}; restore_approval: {restore_approval}",
                file=sys.stderr,
            )
            append_event(run, {"event": "failed", "receipt": str(run / "receipt.json")})
            restore_rows(run, receipt)
            raise
        append_event(run, {"event": "complete", "changed": changed})
        return {
            "changed": changed,
            "receipt": str(run / "receipt.json"),
            "restore_approval": restore_approval,
        }


def restore(home, receipt_path, approval):
    with locked(home) as state:
        path = absolute(receipt_path)
        if (
            path.name != "receipt.json"
            or path.parent.parent != state
            or path.is_symlink()
        ):
            raise ValueError("receipt must belong to this home's installation state")
        private_dir(path.parent)
        try:
            receipt = json.loads(path.read_text())
        except (OSError, ValueError) as exc:
            raise ValueError(f"unreadable receipt: {path}") from exc
        if digest(receipt) != approval:
            raise ValueError("restore approval mismatch")
        plan = receipt["plan"]
        if (
            receipt["version"] != 1
            or receipt["run"] != path.parent.name
            or plan["home"] != str(home)
        ):
            raise ValueError("receipt identity mismatch")
        expected = targets(home, absolute(plan["config_home"]))
        if [str(p) for p in expected] != [r["path"] for r in plan["targets"]]:
            raise ValueError("receipt destinations mismatch")
        count = restore_rows(path.parent, receipt)
        append_event(path.parent, {"event": "restore-complete", "restored": count})
        return {"restored": count, "receipt": str(path)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("plan", "apply"):
        cmd = sub.add_parser(name)
        cmd.add_argument(
            "--source-root",
            required=True,
            help="operator-selected stable ordinary product clone",
        )
        cmd.add_argument("--home", required=True, help="explicit destination home")
        cmd.add_argument(
            "--config-home",
            help="OpenCode config root; defaults to HOME/.config, not ambient XDG",
        )
        cmd.add_argument(
            "--action", choices=("install", "uninstall"), default="install"
        )
        if name == "apply":
            cmd.add_argument(
                "--approve",
                required=True,
                help="exact approval digest from a freshly inspected plan",
            )
    cmd = sub.add_parser("restore")
    cmd.add_argument("--home", required=True)
    cmd.add_argument("--receipt", required=True)
    cmd.add_argument(
        "--approve", required=True, help="restore_approval from apply/prepared event"
    )
    args = parser.parse_args()
    try:
        # Canonicalize HOME once (macOS /var is a system symlink); all children
        # remain lexical and are checked for symlink ancestors before mutation.
        home = Path(args.home).resolve(strict=True)
        if not home.is_dir():
            raise ValueError("home must be an existing directory")
        if args.command == "restore":
            result = restore(home, args.receipt, args.approve)
        else:
            config = (
                absolute(args.config_home) if args.config_home else home / ".config"
            )
            root = Path(args.source_root).resolve(strict=True)
            if args.command == "plan":
                result = make_plan(root, home, config, args.action)
            else:
                result = apply(root, home, config, args.action, args.approve)
        print(json.dumps(result, indent=2, sort_keys=True))
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as exc:
        print(f"skill installation refused: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
