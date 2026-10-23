"""Tests for stringkit."""

from stringkit import slugify


def test_slugify_strips_punctuation():
    assert slugify("Hello, World!") == "hello-world"


def test_slugify_collapses_whitespace():
    assert slugify("  many   spaces  ") == "many-spaces"
