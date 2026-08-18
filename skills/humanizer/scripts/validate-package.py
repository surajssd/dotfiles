#!/usr/bin/env python3
"""Check Humanizer's package files without external dependencies."""

from __future__ import annotations

import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
SKILL = (ROOT / "SKILL.md").read_text(encoding="utf-8")
README = (ROOT / "README.md").read_text(encoding="utf-8")
AGENTS = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
PLUGIN = json.loads((ROOT / ".claude-plugin" / "plugin.json").read_text(encoding="utf-8"))
PLUGIN_SKILL = ROOT / "skills" / "humanizer" / "SKILL.md"


def require_match(match: re.Match[str] | None, message: str) -> re.Match[str]:
    if match is None:
        raise SystemExit(message)
    return match


yaml_metadata = require_match(
    re.match(r"\A---\n(.*?)\n---\n", SKILL, re.DOTALL),
    "SKILL.md must begin with YAML metadata",
).group(1)

for unsupported_field in ("compatibility:", "allowed-tools:"):
    if re.search(rf"(?m)^{re.escape(unsupported_field)}", yaml_metadata):
        raise SystemExit(f"Remove unsupported YAML field: {unsupported_field[:-1]}")

skill_version = require_match(
    re.search(r'(?m)^\s+version:\s*["\']([^"\']+)["\']\s*$', yaml_metadata),
    "Add metadata.version to SKILL.md",
).group(1)
readme_version = require_match(
    re.search(r"(?m)^- \*\*([0-9]+\.[0-9]+\.[0-9]+)\*\*", README),
    "Add a version entry to README.md",
).group(1)

package_versions = {skill_version, readme_version, str(PLUGIN.get("version", ""))}
if len(package_versions) != 1:
    raise SystemExit(
        f"Use one package version in all files: {sorted(package_versions)}"
    )

if not PLUGIN_SKILL.is_symlink():
    raise SystemExit("Link skills/humanizer/SKILL.md to the root SKILL.md")
if PLUGIN_SKILL.resolve() != (ROOT / "SKILL.md").resolve():
    raise SystemExit("Make skills/humanizer/SKILL.md point to the root SKILL.md")

plain_language_rules = (
    "## Writing style",
    "Lead with the main point.",
    "Use common words and active voice.",
    "Keep sentences and paragraphs short.",
    "Use `must` for requirements.",
    "Keep the full technical meaning.",
)
missing_plain_language_rules = [
    rule for rule in plain_language_rules if rule not in AGENTS
]
if missing_plain_language_rules:
    raise SystemExit(
        "Add the missing Plain Language rules to AGENTS.md: "
        + ", ".join(missing_plain_language_rules)
    )

pattern_numbers = [
    int(number)
    for number in re.findall(r"(?m)^### ([0-9]+)\. ", SKILL)
]
if pattern_numbers != list(range(1, 36)):
    raise SystemExit(f"Number SKILL.md patterns from 1 through 35: {pattern_numbers}")

readme_numbers = {
    int(number) for number in re.findall(r"(?m)^\| ([0-9]+) \|", README)
}
if readme_numbers != set(range(1, 36)):
    raise SystemExit("List patterns 1 through 35 in the README table")

if len(SKILL.splitlines()) > 500:
    raise SystemExit("Keep SKILL.md at 500 lines or fewer")

print(f"Humanizer package v{skill_version} is valid")
