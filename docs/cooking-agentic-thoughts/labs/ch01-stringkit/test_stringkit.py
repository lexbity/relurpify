"""Tests for stringkit.

test_slugify_strips_punctuation is supposed to fail in Chapter 1 — it
describes the behavior the agent repairs in Chapter 2.
"""

from stringkit import slugify


def test_slugify_strips_punctuation():
    assert slugify("Hello, World!") == "hello-world"


def test_slugify_collapses_whitespace():
    assert slugify("  many   spaces  ") == "many-spaces"
