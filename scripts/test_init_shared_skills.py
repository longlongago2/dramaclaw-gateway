"""Cross-platform regressions for per-skill links and project-owned skills."""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent


def create_directory_link(source: Path, destination: Path):
    """Create real native links for fixture setup, including legacy layouts."""
    if os.name == "nt":
        from _winapi import CreateJunction

        CreateJunction(str(source), str(destination))
    else:
        destination.symlink_to(source, target_is_directory=True)


class SharedSkillsInitializationTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix="dramaclaw-shared-skills-test-")
        self.workspace = Path(self.scratch.name).resolve()
        self.repo = self.workspace / "checkout with spaces"
        self.repo.mkdir()
        self.shared = self.workspace / ".agents" / "skills"
        self.shared.mkdir(parents=True)
        self.skill = self.shared / "example" / "SKILL.md"
        self.skill.parent.mkdir()
        self.skill.write_text("workspace-owned skill\n", encoding="utf-8")
        self.original = self.skill.read_bytes()
        (self.repo / "scripts").mkdir()
        shutil.copy2(SCRIPTS / "init-shared-skills.py", self.repo / "scripts")
        shutil.copy2(SCRIPTS.parent / ".gitignore", self.repo / ".gitignore")
        self.skills = self.repo / ".agents" / "skills"
        self.skills.mkdir(parents=True)
        (self.skills / ".gitkeep").touch()
        (self.repo / "README.md").write_text("scratch checkout\n", encoding="utf-8")
        self.git("init", "-q", "-b", "main")
        self.git("add", ".agents/skills/.gitkeep", ".gitignore", "README.md", "scripts")
        self.git("commit", "-q", "-m", "placeholder layout")

    def tearDown(self):
        self.scratch.cleanup()

    def remove_skills_placeholder(self):
        (self.skills / ".gitkeep").unlink()
        self.skills.rmdir()

    def git(self, *args):
        return subprocess.check_output(
            [
                "git", "-c", "user.name=Shared Skills Regression",
                "-c", "user.email=skills-test@example.invalid",
                "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false",
                "-c", f"core.hooksPath={self.workspace / 'unused-hooks'}",
                *args,
            ],
            cwd=self.repo, text=True, encoding="utf-8", stderr=subprocess.STDOUT,
        ).strip()

    def initialize(self, *args):
        return subprocess.run(
            [sys.executable, "-X", "utf8", str(self.repo / "scripts" / "init-shared-skills.py"), *args],
            cwd=self.workspace, capture_output=True, text=True, encoding="utf-8",
        )

    def test_initialization_links_individual_skills_and_is_repeatable(self):
        first = self.initialize()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        self.assertEqual(self.skills.resolve(), self.skills)
        self.assertFalse(self.skills.is_symlink())
        if os.name == "nt":
            self.assertFalse(self.skills.lstat().st_file_attributes & stat.FILE_ATTRIBUTE_REPARSE_POINT)
        self.assertEqual((self.skills / "example").resolve(), self.skill.parent)
        second = self.initialize()
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertEqual(self.skill.read_bytes(), self.original)
        self.skill.write_text("shared edit is visible\n", encoding="utf-8")
        self.assertEqual((self.skills / "example" / "SKILL.md").read_bytes(), self.skill.read_bytes())
        (self.repo / ".agents" / "local-config.json").write_text("{}", encoding="utf-8")
        self.git("add", "--all")
        self.assertEqual(self.git("ls-files", "--", ".agents"), ".agents/skills/.gitkeep")
        ignored_directory = subprocess.run(
            ["git", "check-ignore", "-q", ".agents/skills/"], cwd=self.repo,
        )
        self.assertEqual(ignored_directory.returncode, 1)
        self.assertEqual(
            self.git("check-ignore", ".agents/skills/example"),
            ".agents/skills/example",
        )
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_legacy_whole_directory_link_is_migrated_without_changing_source(self):
        self.remove_skills_placeholder()
        create_directory_link(self.shared, self.skills)
        result = self.initialize()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.skills.resolve(), self.skills)
        self.assertTrue((self.skills / ".gitkeep").is_file())
        self.assertEqual((self.skills / "example").resolve(), self.skill.parent)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_project_owned_skills_coexist_even_when_tracked(self):
        project_skill = self.skills / "project-only" / "SKILL.md"
        project_skill.parent.mkdir(parents=True)
        project_skill.write_text("project-owned skill", encoding="utf-8")
        self.git("add", "-f", ".agents/skills/project-only/SKILL.md")
        for _ in range(2):
            result = self.initialize()
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(project_skill.read_text(encoding="utf-8"), "project-owned skill")
        self.assertEqual(project_skill.parent.resolve(), project_skill.parent)
        self.assertEqual((self.skills / "example").resolve(), self.skill.parent)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_name_conflicts_are_rejected_before_creating_any_links(self):
        earlier = self.shared / "aaa-shared"
        earlier.mkdir()
        (earlier / "SKILL.md").write_text("another shared skill", encoding="utf-8")
        local = self.skills / "example" / "SKILL.md"
        local.parent.mkdir(parents=True)
        local.write_text("project version", encoding="utf-8")
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("conflicts with project-owned", result.stderr)
        self.assertFalse((self.skills / "aaa-shared").exists())
        self.assertEqual(local.read_text(encoding="utf-8"), "project version")
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_existing_skill_link_to_other_source_is_preserved(self):
        other = self.workspace / "other skills"
        other.mkdir()
        create_directory_link(other, self.skills / "example")
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("points elsewhere", result.stderr)
        self.assertEqual((self.skills / "example").resolve(), other)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_broken_skill_link_is_preserved_and_rejected(self):
        other = self.workspace / "missing after link creation"
        other.mkdir()
        create_directory_link(other, self.skills / "example")
        other.rmdir()
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("points elsewhere", result.stderr)
        (self.skills / "example").lstat()
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_checkout_with_tracked_shared_skill_is_rejected(self):
        tracked = self.skills / "example" / "SKILL.md"
        tracked.parent.mkdir(parents=True)
        tracked.write_text("legacy tracked skill", encoding="utf-8")
        self.git("add", "-f", ".agents/skills/example/SKILL.md")
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("still tracks", result.stderr)
        self.assertEqual(tracked.read_text(encoding="utf-8"), "legacy tracked skill")
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_missing_shared_source_is_rejected_without_creating_skills(self):
        result = self.initialize("--shared-skills-path", str(self.workspace / "missing"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing", result.stderr)
        self.assertEqual(list(self.skills.iterdir()), [self.skills / ".gitkeep"])
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_custom_external_source_is_supported(self):
        custom = self.workspace / "custom skills"
        custom_skill = custom / "custom-example"
        custom_skill.mkdir(parents=True)
        result = self.initialize("--shared-skills-path", str(custom))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((self.skills / "custom-example").resolve(), custom_skill)
        self.assertFalse((self.skills / "example").exists())
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_source_inside_checkout_is_rejected(self):
        internal = self.repo / "internal"
        internal.mkdir()
        result = self.initialize("--shared-skills-path", str(internal))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("outside this repository", result.stderr)
        self.assertEqual(list(self.skills.iterdir()), [self.skills / ".gitkeep"])
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_source_containing_checkout_is_rejected(self):
        result = self.initialize("--shared-skills-path", str(self.workspace))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("cannot contain", result.stderr)
        self.assertEqual(list(self.skills.iterdir()), [self.skills / ".gitkeep"])
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_existing_whole_directory_link_to_other_source_is_preserved(self):
        self.remove_skills_placeholder()
        other = self.workspace / "other skills"
        other.mkdir()
        create_directory_link(other, self.skills)
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("points elsewhere", result.stderr)
        self.assertEqual(self.skills.resolve(), other)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_project_owned_file_at_skills_path_is_preserved(self):
        self.remove_skills_placeholder()
        self.skills.write_text("project-owned file", encoding="utf-8")
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("project-owned file", result.stderr)
        self.assertEqual(self.skills.read_text(encoding="utf-8"), "project-owned file")

    def test_linked_agents_parent_is_rejected_without_changing_target(self):
        agents = self.repo / ".agents"
        self.remove_skills_placeholder()
        agents.rmdir()
        other = self.workspace / "other agents"
        other.mkdir()
        sentinel = other / "local.txt"
        sentinel.write_text("preserve parent target", encoding="utf-8")
        create_directory_link(other, agents)
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ordinary directory", result.stderr)
        self.assertEqual(sentinel.read_text(encoding="utf-8"), "preserve parent target")
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_new_shared_skills_are_added_on_a_later_run(self):
        first = self.initialize()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        added = self.shared / "added-later"
        added.mkdir()
        (added / "SKILL.md").write_text("new shared skill", encoding="utf-8")
        result = self.initialize()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual((self.skills / "added-later").resolve(), added)
        self.assertEqual((self.skills / "example").resolve(), self.skill.parent)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_switching_branches_preserves_links_and_project_owned_skills(self):
        result = self.initialize()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        project = self.skills / "project-only"
        project.mkdir()
        (project / "SKILL.md").write_text("local skill", encoding="utf-8")
        self.git("switch", "-q", "-c", "without-placeholder")
        self.git("rm", "-q", ".agents/skills/.gitkeep", ".gitignore")
        self.git("commit", "-q", "-m", "checkout without placeholder")
        self.git("switch", "-q", "main")
        self.git("switch", "-q", "without-placeholder")
        self.git("switch", "-q", "main")
        self.assertEqual(self.skills.resolve(), self.skills)
        self.assertEqual((self.skills / "example").resolve(), self.skill.parent)
        self.assertEqual((project / "SKILL.md").read_text(encoding="utf-8"), "local skill")
        self.assertEqual(self.skill.read_bytes(), self.original)
        self.assertEqual(self.git("status", "--porcelain"), "")


if __name__ == "__main__":
    unittest.main(verbosity=2)
