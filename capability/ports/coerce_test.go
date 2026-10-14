package ports

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoerceParameterValue_NilValueAlwaysFails(t *testing.T) {
	for _, paramType := range []ToolParameterType{
		ToolParamString, ToolParamInteger, ToolParamNumber, ToolParamBoolean, ToolParamArray,
	} {
		val, err := CoerceParameterValue(ToolParameter{Name: "p", Type: paramType}, nil)
		require.Error(t, err, "paramType %s", paramType)
		assert.Nil(t, val, "paramType %s", paramType)
	}
}

func TestCoerceParameterValue_String(t *testing.T) {
	param := ToolParameter{Name: "p", Type: ToolParamString}

	t.Run("string passes through", func(t *testing.T) {
		val, err := CoerceParameterValue(param, "hello")
		require.NoError(t, err)
		assert.Equal(t, "hello", val)
	})
	t.Run("integer formats to string", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 42)
		require.NoError(t, err)
		assert.Equal(t, "42", val)
	})
	t.Run("float formats to string", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 1.5)
		require.NoError(t, err)
		assert.Equal(t, "1.5", val)
	})
	t.Run("bool formats to string", func(t *testing.T) {
		val, err := CoerceParameterValue(param, true)
		require.NoError(t, err)
		assert.Equal(t, "true", val)
	})
}

func TestCoerceParameterValue_Integer(t *testing.T) {
	param := ToolParameter{Name: "p", Type: ToolParamInteger}

	t.Run("int64 passes through", func(t *testing.T) {
		val, err := CoerceParameterValue(param, int64(7))
		require.NoError(t, err)
		assert.Equal(t, int64(7), val)
	})
	t.Run("int widens to int64", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 7)
		require.NoError(t, err)
		assert.Equal(t, int64(7), val)
	})
	t.Run("exact float narrows", func(t *testing.T) {
		val, err := CoerceParameterValue(param, float64(7))
		require.NoError(t, err)
		assert.Equal(t, int64(7), val)
	})
	t.Run("lossy float rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 7.5)
		require.Error(t, err)
		assert.Nil(t, val)
		assert.Contains(t, err.Error(), "lossy")
	})
	t.Run("numeric string parses", func(t *testing.T) {
		val, err := CoerceParameterValue(param, " 42 ")
		require.NoError(t, err)
		assert.Equal(t, int64(42), val)
	})
	t.Run("non-numeric string rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, "abc")
		require.Error(t, err)
		assert.Nil(t, val)
	})
	t.Run("bool rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, true)
		require.Error(t, err)
		assert.Nil(t, val)
		assert.Contains(t, err.Error(), "bool")
	})
}

func TestCoerceParameterValue_Number(t *testing.T) {
	param := ToolParameter{Name: "p", Type: ToolParamNumber}

	t.Run("float64 passes through", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 1.25)
		require.NoError(t, err)
		assert.Equal(t, 1.25, val)
	})
	t.Run("int64 widens", func(t *testing.T) {
		val, err := CoerceParameterValue(param, int64(3))
		require.NoError(t, err)
		assert.Equal(t, float64(3), val)
	})
	t.Run("int widens", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 3)
		require.NoError(t, err)
		assert.Equal(t, float64(3), val)
	})
	t.Run("numeric string parses", func(t *testing.T) {
		val, err := CoerceParameterValue(param, " 2.5 ")
		require.NoError(t, err)
		assert.Equal(t, 2.5, val)
	})
	t.Run("non-numeric string rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, "2.5.6")
		require.Error(t, err)
		assert.Nil(t, val)
	})
	t.Run("bool rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, false)
		require.Error(t, err)
		assert.Nil(t, val)
	})
}

func TestCoerceParameterValue_Boolean(t *testing.T) {
	param := ToolParameter{Name: "p", Type: ToolParamBoolean}

	truthy := []string{"true", "TRUE", "1", "yes", " Yes "}
	for _, input := range truthy {
		val, err := CoerceParameterValue(param, input)
		require.NoError(t, err, "input %q", input)
		require.IsType(t, true, val, "input %q", input)
		assert.True(t, val.(bool), "input %q", input)
	}
	falsy := []string{"false", "False", "0", "no", " NO "}
	for _, input := range falsy {
		val, err := CoerceParameterValue(param, input)
		require.NoError(t, err, "input %q", input)
		require.IsType(t, true, val, "input %q", input)
		assert.False(t, val.(bool), "input %q", input)
	}

	t.Run("bool passes through", func(t *testing.T) {
		val, err := CoerceParameterValue(param, true)
		require.NoError(t, err)
		assert.Equal(t, true, val)
	})
	t.Run("unrecognized string rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, "maybe")
		require.Error(t, err)
		assert.Nil(t, val)
	})
	t.Run("int rejected", func(t *testing.T) {
		val, err := CoerceParameterValue(param, 1)
		require.Error(t, err)
		assert.Nil(t, val)
		assert.Contains(t, err.Error(), "int")
	})
}

func TestCoerceParameterValue_UndeclaredTypePassesValueThrough(t *testing.T) {
	for _, paramType := range []ToolParameterType{ToolParamArray, ToolParamObject, "mystery"} {
		val, err := CoerceParameterValue(ToolParameter{Name: "p", Type: paramType}, map[string]any{"k": "v"})
		require.NoError(t, err, "paramType %s", paramType)
		assert.Equal(t, map[string]any{"k": "v"}, val, "paramType %s", paramType)
	}
}

func FuzzCoerceParameterValue(f *testing.F) {
	f.Add("42", "integer")
	f.Add("7.5", "integer")
	f.Add("2.5", "number")
	f.Add("true", "boolean")
	f.Add("maybe", "boolean")
	f.Add("hello", "string")

	f.Fuzz(func(t *testing.T, input string, paramType string) {
		// The coercion of any string must be total: either a value or an
		// error, never a panic — regardless of the declared parameter type.
		val, err := CoerceParameterValue(ToolParameter{Name: "p", Type: ToolParameterType(paramType)}, input)
		if err != nil {
			require.Nil(t, val, "failed coercion must not return a value")
			require.NotEmpty(t, err.Error())
			return
		}
		require.NotNil(t, val, "successful coercion of a non-nil input must return a value")
		// String coercion of any input always succeeds and round-trips.
		if paramType == "string" {
			asString, ok := val.(string)
			require.True(t, ok, "string coercion must return a string, got %T", val)
			require.Equal(t, input, asString)
		}
	})
}
