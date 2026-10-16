#!/usr/bin/env python3
"""Link individual shared skills into a real, project-owned skills directory."""

from __future__ import annotations

import argparse
import os
from pathlib import Path
import stat
import subprocess
import sys


def is_link(path: Path) -> bool:
    """Include Windows Junctions and dangling links without following them."""
    try:
        info = path.lstat()
    except FileNotFoundError:
        return False
    return stat.S_ISLNK(info.st_mode) or bool(
        getattr(info, "st_file_attributes", 0)
        & getattr(stat, "FILE_ATTRIBUTE_REPARSE_POINT", 0)
    )


def initialize_shared_skills(repository: Path, shared: Path) -> str:
    repository = repository.resolve(strict=True)
    try:
        source = shared.expanduser().resolve(strict=True)
    except FileNotFoundError as exc:
        raise ValueError(f"Shared skills directory is missing: {shared}") from exc
    if not source.is_dir():
        raise ValueError(f"Shared skills source is not a directory: {source}")
    if source.is_relative_to(repository) or repository.is_relative_to(source):
        raise ValueError("Shared skills must live outside this repository and cannot contain it.")

    agents = repository / ".agents"
    skills = agents / "skills"
    if is_link(agents) or (agents.exists() and not agents.is_dir()):
        raise ValueError(".agents must be an ordinary directory; its contents were preserved.")

    legacy_link = is_link(skills)
    if legacy_link:
        try:
            same_source = skills.resolve(strict=True) == source
        except (OSError, RuntimeError):
            same_source = False
        if not same_source:
            raise ValueError(f"Existing skills link points elsewhere and was preserved: {skills}")
        if not skills.is_symlink() and (
            os.name != "nt"
            or skills.lstat().st_reparse_tag != stat.IO_REPARSE_TAG_MOUNT_POINT
        ):
            raise ValueError(f"Unsupported reparse point was preserved: {skills}")
    elif skills.exists() and not skills.is_dir():
        raise ValueError(f"A project-owned file blocks the skills directory: {skills}")

    shared_skills = []
    for entry in sorted(source.iterdir(), key=lambda item: item.name):
        if not entry.is_dir() or entry.name.startswith(".") or entry.name == "__pycache__":
            continue
        target = entry.resolve(strict=True)
        if target.is_relative_to(repository) or repository.is_relative_to(target):
            raise ValueError(f"Shared skill must live outside this repository: {entry}")
        shared_skills.append((entry.name, target))

    tracked = subprocess.check_output(
        ["git", "-C", str(repository), "ls-files", "-z", "--", ".agents/skills"],
        stderr=subprocess.PIPE,
    )
    tracked_paths = [
        os.path.normcase(path.decode("utf-8", errors="surrogateescape")).replace("\\", "/")
        for path in tracked.split(b"\0") if path
    ]
    shared_prefixes = [
        os.path.normcase(f".agents/skills/{name}").replace("\\", "/")
        for name, _ in shared_skills
    ]
    if ".agents/skills" in tracked_paths or any(
        path == prefix or path.startswith(prefix + "/")
        for path in tracked_paths for prefix in shared_prefixes
    ):
        raise ValueError("This checkout still tracks shared skill paths; update it before initializing.")

    # Check every name before making any change, preserving project-owned skills.
    missing = []
    for name, target in shared_skills:
        destination = skills / name
        if not legacy_link:
            if is_link(destination):
                try:
                    same_target = destination.resolve(strict=True) == target
                except (OSError, RuntimeError):
                    same_target = False
                if not same_target:
                    raise ValueError(f"Existing skill link points elsewhere and was preserved: {destination}")
                continue
            if destination.exists():
                raise ValueError(f"Shared skill name conflicts with project-owned content: {destination}")
        missing.append((destination, target))

    agents.mkdir(exist_ok=True)
    if legacy_link:
        # Remove only the old link, never its external source or contents.
        if skills.is_symlink():
            skills.unlink()
        else:
            skills.rmdir()
    skills.mkdir(exist_ok=True)
    if is_link(agents) or is_link(skills):
        raise ValueError("Directory layout changed during initialization; no skill links were created.")

    for destination, target in missing:
        if os.name == "nt":
            from _winapi import CreateJunction

            CreateJunction(str(target), str(destination))
        else:
            destination.symlink_to(target, target_is_directory=True)

    return (
        f"Shared skills: created {len(missing)}, reused {len(shared_skills) - len(missing)}. "
        f"Project skills directory preserved: {skills}"
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--shared-skills-path", type=Path,
        help="shared source directory (default: ../.agents/skills relative to this checkout)",
    )
    args = parser.parse_args()
    repository = Path(__file__).resolve().parents[1]
    shared = args.shared_skills_path
    if shared is None:
        shared = repository.parent / ".agents" / "skills"
    try:
        print(initialize_shared_skills(repository, shared))
    except (ImportError, OSError, RuntimeError, ValueError, subprocess.CalledProcessError) as exc:
        print(f"Could not initialize shared skills: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
