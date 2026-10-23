"""stringkit — small string utilities, kept tiny on purpose.

Chapter 1 of *Cooking Agentic Thoughts with Relurpify* seeds this module
with a deliberately buggy slugify: it lowercases and joins words with
hyphens, but it keeps punctuation. One test fails on purpose; the agent
fixes it in Chapter 2.
"""

import re


def slugify(text: str) -> str:
    """Turn a title into a URL-safe slug."""
    slug = text.strip().lower()
    slug = re.sub(r"\s+", "-", slug)
    return slug
