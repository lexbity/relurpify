package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveNativeRegistry snapshots the process-wide registry so tests can mutate
// it freely and restore the exact prior contents afterwards (hermetic).
func saveNativeRegistry(t *testing.T) {
	t.Helper()
	snapshot := DefaultNativeConstructors()
	t.Cleanup(func() {
		ResetNativeRegistry()
		for key, ctor := range snapshot {
			if err := RegisterNativeNoPanic(key, ctor); err != nil {
				t.Fatalf("restoring native registry key %q: %v", key, err)
			}
		}
	})
}

// noopConstructor is a constructor that always yields a fresh fakeTool.
func noopConstructor() NativeToolConstructor {
	return func(basePath string) Tool {
		return &fakeTool{name: "constructed:" + basePath}
	}
}

func TestRegisterNativeNoPanic_RegistersNormalizedKey(t *testing.T) {
	saveNativeRegistry(t)

	require.NoError(t, RegisterNativeNoPanic("Ports-Test.Widget", noopConstructor()))

	ctor, ok := LookupNative("ports_test_widget")
	require.True(t, ok, "lookup must find the constructor under the normalized key")
	require.NotNil(t, ctor)

	// Any alias form of the key resolves to the same constructor.
	_, ok = LookupNative("  PORTS TEST/WIDGET ")
	assert.True(t, ok)

	// The constructor itself builds a working tool.
	tool := ctor("/base")
	assert.Equal(t, "constructed:/base", tool.Name())
}

func TestRegisterNativeNoPanic_RejectsInvalidRegistrations(t *testing.T) {
	saveNativeRegistry(t)

	require.NoError(t, RegisterNativeNoPanic("ports-test-dup", noopConstructor()))

	cases := []struct {
		name string
		key  string
		ctor NativeToolConstructor
	}{
		{"empty key", "", noopConstructor()},
		{"whitespace-only key", "   ", noopConstructor()},
		{"punctuation-only key", "!!", noopConstructor()},
		{"nil constructor", "ports-test-nil-ctor", nil},
		{"duplicate key", "ports_test_dup", noopConstructor()},
		{"duplicate via alias", "PORTS-TEST.DUP", noopConstructor()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RegisterNativeNoPanic(tc.key, tc.ctor)
			require.Error(t, err)
			assert.NotEmpty(t, err.Error())
		})
	}
}

func TestRegisterNative_PanicsOnDuplicateAndEmptyKey(t *testing.T) {
	saveNativeRegistry(t)

	require.NotPanics(t, func() {
		RegisterNative("ports-test-panic-fresh", noopConstructor())
	}, "first registration of a fresh key must not panic")

	require.Panics(t, func() {
		RegisterNative("ports test panic fresh", noopConstructor())
	}, "re-registering the same normalized key must panic")

	require.Panics(t, func() {
		RegisterNative("", noopConstructor())
	}, "registering an empty key must panic")
}

func TestLookupNative_UnknownKeyReturnsFalse(t *testing.T) {
	saveNativeRegistry(t)

	ctor, ok := LookupNative("ports-test-never-registered")
	assert.False(t, ok)
	assert.Nil(t, ctor)

	// Keys that normalize to nothing can never be registered, so lookup fails.
	ctor, ok = LookupNative("  ")
	assert.False(t, ok)
	assert.Nil(t, ctor)
}

func TestDefaultNativeConstructors_ReturnsIndependentSnapshot(t *testing.T) {
	saveNativeRegistry(t)
	require.NoError(t, RegisterNativeNoPanic("ports-test-snapshot", noopConstructor()))

	snapshot := DefaultNativeConstructors()
	require.Contains(t, snapshot, "ports_test_snapshot")

	// Mutating the returned map must not affect the registry itself.
	delete(snapshot, "ports_test_snapshot")
	_, ok := LookupNative("ports_test_snapshot")
	assert.True(t, ok, "DefaultNativeConstructors must hand out a copy, not the live map")
}

func TestNativeKeys_SortedAndComplete(t *testing.T) {
	saveNativeRegistry(t)
	require.NoError(t, RegisterNativeNoPanic("ports-test-keys-b", noopConstructor()))
	require.NoError(t, RegisterNativeNoPanic("ports-test-keys-a", noopConstructor()))

	keys := NativeKeys()
	require.GreaterOrEqual(t, len(keys), 2)
	assert.Contains(t, keys, "ports_test_keys_a")
	assert.Contains(t, keys, "ports_test_keys_b")
	assert.IsNonDecreasing(t, keys, "NativeKeys must return keys in sorted order")
}

func TestResetNativeRegistry_ClearsAllConstructors(t *testing.T) {
	saveNativeRegistry(t)
	require.NoError(t, RegisterNativeNoPanic("ports-test-reset", noopConstructor()))

	ResetNativeRegistry()

	assert.Empty(t, DefaultNativeConstructors())
	_, ok := LookupNative("ports_test_reset")
	assert.False(t, ok)
	assert.Empty(t, NativeKeys())
}

func TestNormalizeToolName_Internal(t *testing.T) {
	cases := map[string]string{
		"File-Read":     "file_read",
		"FILE.READ":     "file_read",
		"file/read":     "file_read",
		"file read":     "file_read",
		"  spaced  ":    "spaced",
		"a  b":          "a_b",
		"_leading_":     "leading",
		"-dash-":        "dash",
		"abc123":        "abc123",
		"!!":            "",
		"":              "",
		"   ":           "",
		"mixed_KEBAB.X": "mixed_kebab_x",
	}
	for input, want := range cases {
		assert.Equal(t, want, normalizeToolName(input), "normalizeToolName(%q)", input)
	}
}
