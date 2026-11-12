#!/usr/bin/env python3
"""Convert Markdown documentation into Hugo-compatible content.

Reads every ``.md`` file from a source directory (default: this script's
directory) and writes it to ``content/docs/`` at the repository root with
Hugo front matter prepended.

Organization:
  * Top-level ``.md`` files are categorized by filename keywords into
    ``getting-started/``, ``guides/``, ``reference/`` or ``miscellaneous/``.
  * Every subdirectory of the source is treated as a book: its files are
    copied with their relative structure intact (including nested ``labs``
    folders) so each book keeps its own layout.

Weight:
  * Categorized files get ``10 + (category_index * 5)`` where the category
    index follows the order getting-started, guides, reference, miscellaneous.
  * Book files get ``10 + (index * 5)`` ordered by their relative path inside
    the book, so chapters (``ch01-...``) render in reading order.
"""

from __future__ import annotations

import argparse
import re
import sys
from datetime import datetime, timezone
from pathlib import Path

# Category rules, checked in order against the lowercased filename. Keywords
# are stems so variants match too ("configur" covers configure/configuration).
CATEGORY_RULES = [
    ("getting-started", ("readme", "intro", "overview", "install", "setup", "configur", "start")),
    ("guides", ("usage", "guide", "tutorial")),
    ("reference", ("api", "reference", "docs")),
]
CATEGORY_WEIGHTS = {"getting-started": 10, "guides": 15, "reference": 20, "miscellaneous": 25}

FRONT_MATTER_RE = re.compile(r"\A---\s*\n.*?\n---\s*\n?", re.DOTALL)
HEADING_RE = re.compile(r"^#\s+(.+?)\s*$", re.MULTILINE)


def categorize(filename: str) -> str:
    name = filename.lower()
    for category, keywords in CATEGORY_RULES:
        if any(keyword in name for keyword in keywords):
            return category
    return "miscellaneous"


def extract_title(content: str, path: Path) -> str:
    match = HEADING_RE.search(content)
    if match:
        return match.group(1).strip()
    stem = path.stem.replace("-", " ").replace("_", " ")
    return stem.title() if stem.islower() else stem


def project_name(source: Path) -> str:
    # The source directory lives inside the repository root; the repo's name
    # is the project name used in every title.
    parent = source.parent if source.is_absolute() else source.resolve().parent
    return parent.name


def lastmod(path: Path) -> str:
    mtime = datetime.fromtimestamp(path.stat().st_mtime, tz=timezone.utc)
    return mtime.strftime("%Y-%m-%dT%H:%M:%S+00:00")


def front_matter(title: str, weight: int, modified: str) -> str:
    return (
        "---\n"
        f'title: "{title}"\n'
        f"weight: {weight}\n"
        "type: docs\n"
        "draft: false\n"
        f"lastmod: {modified}\n"
        "---\n\n"
    )


def collect(source: Path) -> list[tuple[Path, Path, str]]:
    """Return (source_file, output_file, book_name) triples.

    Top-level files are categorized; files inside subdirectories belong to
    that book and keep their relative path.
    """
    entries = []
    for path in sorted(source.rglob("*.md")):
        rel = path.relative_to(source)
        depth = len(rel.parts)
        if depth == 1:
            out_rel = Path(categorize(rel.name)) / rel.name
            book = ""
        else:
            book = rel.parts[0]
            out_rel = Path(*rel.parts)
        entries.append((path, out_rel, book))
    return entries


def export(source: Path, output: Path) -> int:
    project = project_name(source)
    entries = collect(source)
    if not entries:
        print(f"error: no markdown files found under {source}", file=sys.stderr)
        return 1

    book_indexes: dict[str, int] = {}
    written = 0
    for path, out_rel, book in entries:
        content = path.read_text(encoding="utf-8")
        content = FRONT_MATTER_RE.sub("", content, count=1)

        title = f"{project} - {extract_title(content, path)}"
        if book:
            index = book_indexes.get(book, 0)
            book_indexes[book] = index + 1
            weight = 10 + (index * 5)
        else:
            weight = CATEGORY_WEIGHTS[categorize(path.name)]

        dest = output / out_rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(front_matter(title, weight, lastmod(path)) + content, encoding="utf-8")
        written += 1
        print(f"{path} -> {dest}")

    print(f"\nexported {written} file(s) to {output}")
    return 0


def main() -> int:
    script_dir = Path(__file__).resolve().parent
    repo_root = script_dir.parent
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--source", type=Path, default=script_dir,
                        help="directory containing the markdown docs (default: %(default)s)")
    parser.add_argument("--output", type=Path, default=repo_root / "content" / "docs",
                        help="Hugo content output directory (default: %(default)s)")
    args = parser.parse_args()
    if not args.source.is_dir():
        print(f"error: source directory not found: {args.source}", file=sys.stderr)
        return 1
    return export(args.source, args.output)


if __name__ == "__main__":
    sys.exit(main())
