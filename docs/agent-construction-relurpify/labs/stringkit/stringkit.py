"""stringkit — small string utilities, kept tiny on purpose.

Reset for *Agent Construction with Relurpify* (book 2): slugify strips
punctuation, collapses whitespace, and both tests pass. The critic you
build in this book will read and critique code against this library.

Lineage: the same demo repo book 1 (*Cooking Agentic Thoughts with
Relurpify*) seeded with a deliberately buggy slugify. Book 2 starts from
the repaired state; it does not carry book 1's seeded failure.
"""

import re


def slugify(text: str) -> str:
    """Turn a title into a URL-safe slug."""
    slug = text.strip().lower()
    slug = re.sub(r"[^\w\s]+", "", slug)
    slug = re.sub(r"\s+", "-", slug)
    return slug
