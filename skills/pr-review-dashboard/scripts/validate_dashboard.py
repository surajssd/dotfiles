#!/usr/bin/env python3
"""Validate a generated PR review dashboard or its bundled template."""

from __future__ import annotations

import argparse
import re
import sys
from collections import Counter
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlparse


REQUIRED_TABS = (
    "summary",
    "findings",
    "architecture",
    "diff",
    "verification",
    "glossary",
    "unknowns",
)
PINNED_MERMAID = "https://cdn.jsdelivr.net/npm/mermaid@10.9.6/dist/mermaid.min.js"
VOID_ELEMENTS = {
    "area",
    "base",
    "br",
    "col",
    "embed",
    "hr",
    "img",
    "input",
    "link",
    "meta",
    "param",
    "source",
    "track",
    "wbr",
}


class DashboardParser(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.ids: list[str] = []
        self.tabs: list[dict[str, str]] = []
        self.panels: list[dict[str, str]] = []
        self.resource_urls: list[tuple[str, str]] = []
        self.internal_anchors: list[str] = []
        self.blank_links_without_rel: list[str] = []
        self.classes: Counter[str] = Counter()
        self.comments: list[str] = []
        self.text: list[str] = []
        self.stack: list[str] = []
        self.structure_errors: list[str] = []
        self.has_viewport = False
        self.has_csp = False
        self.html_lang = ""

    def handle_decl(self, decl: str) -> None:
        return

    def handle_comment(self, data: str) -> None:
        self.comments.append(data)

    def handle_data(self, data: str) -> None:
        self.text.append(data)

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        values = {key: value or "" for key, value in attrs}
        if tag not in VOID_ELEMENTS:
            self.stack.append(tag)

        if values.get("id"):
            self.ids.append(values["id"])

        class_names = values.get("class", "").split()
        self.classes.update(class_names)

        if values.get("role") == "tab":
            self.tabs.append(values)
        if values.get("role") == "tabpanel":
            self.panels.append(values)

        if tag == "html":
            self.html_lang = values.get("lang", "")
        if tag == "meta" and values.get("name", "").lower() == "viewport":
            self.has_viewport = bool(values.get("content"))
        if tag == "meta" and values.get("http-equiv", "").lower() == "content-security-policy":
            self.has_csp = "default-src 'none'" in values.get("content", "")

        if tag in {"script", "img", "link", "iframe", "source"}:
            attribute = "href" if tag == "link" else "src"
            if values.get(attribute):
                self.resource_urls.append((tag, values[attribute]))

        if tag == "a" and values.get("href", "").startswith("#") and len(values["href"]) > 1:
            self.internal_anchors.append(values["href"][1:])

        if tag == "a" and values.get("target") == "_blank":
            rel = set(values.get("rel", "").split())
            if not {"noopener", "noreferrer"}.issubset(rel):
                self.blank_links_without_rel.append(values.get("href", "<missing href>"))

    def handle_startendtag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        self.handle_starttag(tag, attrs)
        if tag not in VOID_ELEMENTS:
            self.handle_endtag(tag)

    def handle_endtag(self, tag: str) -> None:
        if tag in VOID_ELEMENTS:
            return
        if not self.stack:
            self.structure_errors.append(f"unexpected closing tag </{tag}>")
            return
        if self.stack[-1] == tag:
            self.stack.pop()
            return
        if tag not in self.stack:
            self.structure_errors.append(f"closing tag </{tag}> has no matching opener")
            return
        while self.stack and self.stack[-1] != tag:
            unclosed = self.stack.pop()
            self.structure_errors.append(f"<{unclosed}> is not closed before </{tag}>")
        if self.stack:
            self.stack.pop()


def validate(path: Path, template_mode: bool) -> tuple[list[str], list[str]]:
    errors: list[str] = []
    warnings: list[str] = []

    try:
        source = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        return [f"cannot read UTF-8 HTML: {exc}"], warnings

    if not re.match(r"\s*<!doctype\s+html>", source, flags=re.IGNORECASE):
        errors.append("missing <!doctype html>")

    parser = DashboardParser()
    try:
        parser.feed(source)
        parser.close()
    except Exception as exc:  # HTMLParser reports malformed declarations here.
        errors.append(f"HTML parser error: {exc}")
        return errors, warnings

    if parser.stack:
        errors.append("unclosed elements at EOF: " + ", ".join(f"<{tag}>" for tag in parser.stack[-8:]))
    errors.extend(parser.structure_errors)

    duplicate_ids = sorted(name for name, count in Counter(parser.ids).items() if count > 1)
    if duplicate_ids:
        errors.append("duplicate id attributes: " + ", ".join(duplicate_ids))

    known_ids = set(parser.ids)
    dangling_anchors = sorted({name for name in parser.internal_anchors if name not in known_ids})
    if dangling_anchors:
        errors.append("internal anchors point to missing ids: " + ", ".join(f"#{name}" for name in dangling_anchors))

    if parser.html_lang != "en":
        errors.append("root html element must declare lang=\"en\"")
    if not parser.has_viewport:
        errors.append("missing viewport meta tag")
    if not parser.has_csp:
        errors.append("missing restrictive Content Security Policy")

    tab_names = [tab.get("data-tab", "") for tab in parser.tabs]
    panel_names = [panel.get("id", "").removeprefix("tab-") for panel in parser.panels]
    if tuple(tab_names) != REQUIRED_TABS:
        errors.append(f"tab order must be {', '.join(REQUIRED_TABS)}; found {', '.join(tab_names) or 'none'}")
    if tuple(panel_names) != REQUIRED_TABS:
        errors.append(f"tabpanel order must be {', '.join(REQUIRED_TABS)}; found {', '.join(panel_names) or 'none'}")

    panel_by_name = {panel.get("id", "").removeprefix("tab-"): panel for panel in parser.panels}
    for tab in parser.tabs:
        name = tab.get("data-tab", "")
        expected_panel = f"tab-{name}"
        expected_button = f"tab-button-{name}"
        if tab.get("id") != expected_button:
            errors.append(f"tab {name!r} must use id {expected_button!r}")
        if tab.get("aria-controls") != expected_panel:
            errors.append(f"tab {name!r} must control {expected_panel!r}")
        panel = panel_by_name.get(name)
        if panel and panel.get("aria-labelledby") != expected_button:
            errors.append(f"panel {name!r} must be labelled by {expected_button!r}")

    active_tabs = [tab for tab in parser.tabs if "active" in tab.get("class", "").split()]
    if len(active_tabs) != 1 or active_tabs[0].get("data-tab") != "summary":
        errors.append("exactly the Summary tab must be initially active")

    for url in parser.blank_links_without_rel:
        errors.append(f'target="_blank" link lacks rel="noopener noreferrer": {url}')

    mermaid_scripts = [url for tag, url in parser.resource_urls if tag == "script" and "mermaid" in url.lower()]
    for tag, url in parser.resource_urls:
        parsed = urlparse(url)
        if parsed.scheme in {"http", "https"} and not (tag == "script" and url == PINNED_MERMAID):
            errors.append(f"unapproved external resource: <{tag}> {url}")
    if mermaid_scripts and mermaid_scripts != [PINNED_MERMAID]:
        errors.append(f"Mermaid must use the exact pinned URL {PINNED_MERMAID}")

    fill_markers = [comment.strip() for comment in parser.comments if "FILL:" in comment]
    if template_mode:
        required_markers = (
            "FILL: PR title",
            "FILL: actionable findings",
            "FILL: zero or more evidence-backed diagram cards",
            "FILL: consequential hunks",
            "FILL: exact commands",
            "FILL: honest, specific list",
        )
        for marker in required_markers:
            if marker not in source:
                errors.append(f"template is missing marker: {marker}")
    elif fill_markers or "FILL:" in source:
        errors.append(f"unresolved FILL markers remain ({len(fill_markers)} comments)")

    if not template_mode:
        text = " ".join(parser.text)
        if not parser.classes["finding"] and not (parser.classes["no-findings"] and "No actionable findings" in text):
            errors.append("add at least one .finding or an explicit .no-findings result")

        approximate_locations = re.findall(
            r"(?:\blines?\b|\bL\b|[:#])\s*(?:[:=]\s*)?[~≈]\s*\d+",
            text,
            flags=re.IGNORECASE,
        )
        if approximate_locations:
            errors.append("approximate code locations are forbidden: " + ", ".join(sorted(set(approximate_locations))))

        mermaid_blocks = parser.classes["mermaid"]
        if mermaid_blocks and not mermaid_scripts:
            errors.append("Mermaid blocks exist but the pinned Mermaid script is missing")
        if not mermaid_blocks and mermaid_scripts:
            warnings.append("remove the Mermaid script because the dashboard has no Mermaid blocks")

    if "securityLevel: 'strict'" not in source or "htmlLabels: false" not in source:
        errors.append("Mermaid initialization must keep strict security and HTML labels disabled")
    if "aria-selected" not in source or "ArrowRight" not in source:
        errors.append("accessible tab state or keyboard navigation wiring is missing")

    return errors, warnings


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("html", type=Path, help="dashboard HTML file")
    parser.add_argument("--template", action="store_true", help="allow and validate FILL markers")
    args = parser.parse_args()

    errors, warnings = validate(args.html, args.template)
    for warning in warnings:
        print(f"WARNING: {warning}", file=sys.stderr)
    for error in errors:
        print(f"ERROR: {error}", file=sys.stderr)

    if errors:
        print(f"Dashboard validation failed with {len(errors)} error(s).", file=sys.stderr)
        return 1
    print(f"Dashboard validation passed: {args.html}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
