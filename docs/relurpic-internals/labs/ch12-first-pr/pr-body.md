# drop dangling design-doc reference in grounding comment

**What:** drop a dangling design-doc reference from the
`CaptureKindForOrigin` comment (3 lines, comment-only).

**Why:** the referenced plan doc
(`devdocs/plans/bkc-bidirectional-context-and-hermetic-dryrun-spec.md`)
no longer exists in the tree; repointing at `devdocs/` would dangle for
every public reader, since `devdocs/` is not part of the clone. The
sentence stands without the pointer.

**Gates run:** `go build ./context/...`; `go test ./context/knowledge`;
`make envcheck` (1087 files scanned, PASS).

**Known gaps:** none in this diff. The sibling pathway —
length-prefixing `compilationDigest` so the deterministic digest is
also collision-safe across field boundaries — is drafted separately
(see the internals book, Chapter 8 §8.3 and Chapter 12 §12.5, rung 2).

Signed-off-by: prestin liu <lexbit@users.noreply.github.com>
