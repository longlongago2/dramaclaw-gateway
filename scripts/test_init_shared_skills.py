"""Windows regressions for shared skills initialization and Git checkout safety."""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent
POWERSHELL = shutil.which("powershell.exe")


@unittest.skipUnless(os.name == "nt" and POWERSHELL, "Windows Junctions require PowerShell")
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
        shutil.copy2(SCRIPTS / "init-shared-skills.ps1", self.repo / "scripts")
        shutil.copy2(SCRIPTS.parent / ".gitignore", self.repo / ".gitignore")
        (self.repo / ".agents").mkdir()
        (self.repo / ".agents" / ".gitkeep").touch()
        (self.repo / "README.md").write_text("scratch checkout\n", encoding="utf-8")
        self.git("init", "-q", "-b", "main")
        self.git("add", ".agents/.gitkeep", ".gitignore", "README.md", "scripts")
        self.git("commit", "-q", "-m", "placeholder layout")
        self.link = self.repo / ".agents" / "skills"

    def tearDown(self):
        self.scratch.cleanup()

    def git(self, *args):
        return subprocess.check_output(
            [
                "git", "-c", "user.name=Junction Regression",
                "-c", "user.email=junction-test@example.invalid",
                "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false",
                "-c", f"core.hooksPath={self.workspace / 'unused-hooks'}",
                *args,
            ],
            cwd=self.repo, text=True, encoding="utf-8", stderr=subprocess.STDOUT,
        ).strip()

    def initialize(self, *args):
        return subprocess.run(
            [
                POWERSHELL, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
                "-File", str(self.repo / "scripts" / "init-shared-skills.ps1"), *args,
            ],
            cwd=self.workspace, capture_output=True, text=True, encoding="utf-8",
        )

    def test_initialization_is_repeatable_and_skills_stay_ignored(self):
        first = self.initialize()
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        self.assertEqual(self.link.resolve(), self.shared)
        self.assertTrue(self.link.lstat().st_file_attributes & stat.FILE_ATTRIBUTE_REPARSE_POINT)
        second = self.initialize()
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertEqual(self.skill.read_bytes(), self.original)
        (self.repo / ".agents" / "local-config.json").write_text("{}", encoding="utf-8")
        self.git("add", "--all")
        self.assertEqual(self.git("ls-files", "--", ".agents"), ".agents/.gitkeep")
        self.assertEqual(
            self.git("check-ignore", ".agents/skills/example/SKILL.md"),
            ".agents/skills/example/SKILL.md",
        )
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_switching_branches_preserves_junction_and_shared_files(self):
        result = self.initialize()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.git("switch", "-q", "-c", "without-placeholder")
        self.git("rm", "-q", ".agents/.gitkeep", ".gitignore")
        self.git("commit", "-q", "-m", "checkout without placeholder")
        self.git("switch", "-q", "main")
        self.git("switch", "-q", "without-placeholder")
        self.git("switch", "-q", "main")
        self.assertEqual(self.link.resolve(), self.shared)
        self.assertTrue(self.link.lstat().st_file_attributes & stat.FILE_ATTRIBUTE_REPARSE_POINT)
        self.assertEqual(self.skill.read_bytes(), self.original)
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_nonempty_local_skills_are_preserved_and_rejected(self):
        self.link.mkdir()
        local = self.link / "local.md"
        local.write_text("local work", encoding="utf-8")
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("nonempty", result.stderr)
        self.assertEqual(local.read_text(encoding="utf-8"), "local work")
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_empty_local_skills_directory_can_be_initialized(self):
        self.link.mkdir()
        result = self.initialize()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.link.resolve(), self.shared)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_existing_junction_to_other_source_is_preserved(self):
        other = self.workspace / "other skills"
        other.mkdir()
        first = self.initialize("-SharedSkillsPath", str(other))
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("points elsewhere", result.stderr)
        self.assertEqual(self.link.resolve(), other)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_checkout_with_tracked_skills_is_rejected(self):
        self.link.mkdir()
        tracked = self.link / "tracked.md"
        tracked.write_text("legacy tracked skill", encoding="utf-8")
        self.git("add", "-f", ".agents/skills/tracked.md")
        result = self.initialize()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("still tracks", result.stderr)
        self.assertEqual(tracked.read_text(encoding="utf-8"), "legacy tracked skill")
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_missing_shared_source_is_rejected_without_creating_link(self):
        result = self.initialize("-SharedSkillsPath", str(self.workspace / "missing"))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing", result.stderr)
        self.assertFalse(self.link.exists())
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_custom_external_source_is_supported(self):
        custom = self.workspace / "custom skills"
        custom.mkdir()
        result = self.initialize("-SharedSkillsPath", str(custom))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.link.resolve(), custom)
        self.assertEqual(self.skill.read_bytes(), self.original)

    def test_source_inside_checkout_is_rejected(self):
        internal = self.repo / "internal"
        internal.mkdir()
        result = self.initialize("-SharedSkillsPath", str(internal))
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("outside this repository", result.stderr)
        self.assertFalse(self.link.exists())
        self.assertEqual(self.skill.read_bytes(), self.original)


if __name__ == "__main__":
    unittest.main(verbosity=2)
