package common

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLambdaRuntime(t *testing.T) {
	runtime := NewLambdaRuntime()
	require.NotNil(t, runtime)
}

func TestLambdaRuntimeInfo_GetFunctionName(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
		runtime := NewLambdaRuntime()
		assert.Empty(t, runtime.GetFunctionName())
	})

	t.Run("returns value when set", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-function")
		defer os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

		runtime := NewLambdaRuntime()
		assert.Equal(t, "test-function", runtime.GetFunctionName())
	})
}

func TestLambdaRuntimeInfo_GetFunctionVersion(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_VERSION")
		runtime := NewLambdaRuntime()
		assert.Empty(t, runtime.GetFunctionVersion())
	})

	t.Run("returns value when set", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "$LATEST")
		defer os.Unsetenv("AWS_LAMBDA_FUNCTION_VERSION")

		runtime := NewLambdaRuntime()
		assert.Equal(t, "$LATEST", runtime.GetFunctionVersion())
	})
}

func TestLambdaRuntimeInfo_GetMemorySize(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
		runtime := NewLambdaRuntime()
		assert.Empty(t, runtime.GetMemorySize())
	})

	t.Run("returns value when set", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "512")
		defer os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")

		runtime := NewLambdaRuntime()
		assert.Equal(t, "512", runtime.GetMemorySize())
	})
}

func TestLambdaRuntimeInfo_GetMemorySizeInt(t *testing.T) {
	t.Run("returns 0 when not set", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
		runtime := NewLambdaRuntime()
		size, err := runtime.GetMemorySizeInt()
		require.NoError(t, err)
		assert.Equal(t, 0, size)
	})

	t.Run("returns parsed int when set", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "1024")
		defer os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")

		runtime := NewLambdaRuntime()
		size, err := runtime.GetMemorySizeInt()
		require.NoError(t, err)
		assert.Equal(t, 1024, size)
	})

	t.Run("returns error for invalid value", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "invalid")
		defer os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")

		runtime := NewLambdaRuntime()
		_, err := runtime.GetMemorySizeInt()
		assert.Error(t, err)
	})
}

func TestLambdaRuntimeInfo_GetInitializationType(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_INITIALIZATION_TYPE")
		runtime := NewLambdaRuntime()
		assert.Empty(t, runtime.GetInitializationType())
	})

	t.Run("returns value when set", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_INITIALIZATION_TYPE", "on-demand")
		defer os.Unsetenv("AWS_LAMBDA_INITIALIZATION_TYPE")

		runtime := NewLambdaRuntime()
		assert.Equal(t, "on-demand", runtime.GetInitializationType())
	})
}

func TestLambdaRuntimeInfo_GetLogGroupName(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_LOG_GROUP_NAME")
		runtime := NewLambdaRuntime()
		assert.Empty(t, runtime.GetLogGroupName())
	})

	t.Run("returns value when set", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_LOG_GROUP_NAME", "/aws/lambda/test")
		defer os.Unsetenv("AWS_LAMBDA_LOG_GROUP_NAME")

		runtime := NewLambdaRuntime()
		assert.Equal(t, "/aws/lambda/test", runtime.GetLogGroupName())
	})
}

func TestLambdaRuntimeInfo_GetLogStreamName(t *testing.T) {
	os.Setenv("AWS_LAMBDA_LOG_STREAM_NAME", "2024/01/01/[$LATEST]abc123")
	defer os.Unsetenv("AWS_LAMBDA_LOG_STREAM_NAME")

	runtime := NewLambdaRuntime()
	assert.Equal(t, "2024/01/01/[$LATEST]abc123", runtime.GetLogStreamName())
}

func TestLambdaRuntimeInfo_GetXRayTraceID(t *testing.T) {
	os.Setenv("_X_AMZN_TRACE_ID", "Root=1-abc123;Parent=def456;Sampled=1")
	defer os.Unsetenv("_X_AMZN_TRACE_ID")

	runtime := NewLambdaRuntime()
	assert.Contains(t, runtime.GetXRayTraceID(), "Root=1-abc123")
}

func TestLambdaRuntimeInfo_IsRunningInLambda(t *testing.T) {
	t.Run("false when not in Lambda", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
		runtime := NewLambdaRuntime()
		assert.False(t, runtime.IsRunningInLambda())
	})

	t.Run("true when in Lambda", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "test-func")
		defer os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

		runtime := NewLambdaRuntime()
		assert.True(t, runtime.IsRunningInLambda())
	})
}

func TestLambdaRuntimeInfo_IsXRayEnabled(t *testing.T) {
	t.Run("false when not enabled", func(t *testing.T) {
		os.Unsetenv("_X_AMZN_TRACE_ID")
		runtime := NewLambdaRuntime()
		assert.False(t, runtime.IsXRayEnabled())
	})

	t.Run("true when enabled", func(t *testing.T) {
		os.Setenv("_X_AMZN_TRACE_ID", "Root=1-abc")
		defer os.Unsetenv("_X_AMZN_TRACE_ID")

		runtime := NewLambdaRuntime()
		assert.True(t, runtime.IsXRayEnabled())
	})
}

func TestLambdaRuntimeInfo_IsColdStart(t *testing.T) {
	t.Run("true for on-demand", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_INITIALIZATION_TYPE", "on-demand")
		defer os.Unsetenv("AWS_LAMBDA_INITIALIZATION_TYPE")

		runtime := NewLambdaRuntime()
		assert.True(t, runtime.IsColdStart())
	})

	t.Run("false for provisioned-concurrency", func(t *testing.T) {
		os.Setenv("AWS_LAMBDA_INITIALIZATION_TYPE", "provisioned-concurrency")
		defer os.Unsetenv("AWS_LAMBDA_INITIALIZATION_TYPE")

		runtime := NewLambdaRuntime()
		assert.False(t, runtime.IsColdStart())
	})

	t.Run("true when not set (defaults to cold start)", func(t *testing.T) {
		os.Unsetenv("AWS_LAMBDA_INITIALIZATION_TYPE")
		runtime := NewLambdaRuntime()
		assert.True(t, runtime.IsColdStart())
	})
}

// Test package-level convenience functions
func TestPackageLevelRuntimeFunctions(t *testing.T) {
	// Set up test environment
	os.Setenv("AWS_LAMBDA_FUNCTION_NAME", "pkg-test")
	os.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "v1")
	os.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256")
	os.Setenv("AWS_LAMBDA_INITIALIZATION_TYPE", "on-demand")
	os.Setenv("AWS_LAMBDA_LOG_GROUP_NAME", "/aws/lambda/pkg-test")
	os.Setenv("AWS_LAMBDA_LOG_STREAM_NAME", "stream-1")
	os.Setenv("_X_AMZN_TRACE_ID", "Root=1-xyz")
	defer func() {
		os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
		os.Unsetenv("AWS_LAMBDA_FUNCTION_VERSION")
		os.Unsetenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
		os.Unsetenv("AWS_LAMBDA_INITIALIZATION_TYPE")
		os.Unsetenv("AWS_LAMBDA_LOG_GROUP_NAME")
		os.Unsetenv("AWS_LAMBDA_LOG_STREAM_NAME")
		os.Unsetenv("_X_AMZN_TRACE_ID")
	}()

	assert.Equal(t, "pkg-test", GetLambdaFunctionName())
	assert.Equal(t, "v1", GetLambdaFunctionVersion())
	assert.Equal(t, "256", GetLambdaMemorySize())

	memSize, err := GetLambdaMemorySizeInt()
	require.NoError(t, err)
	assert.Equal(t, 256, memSize)

	assert.Equal(t, "on-demand", GetLambdaInitializationType())
	assert.Equal(t, "/aws/lambda/pkg-test", GetLambdaLogGroupName())
	assert.Equal(t, "stream-1", GetLambdaLogStreamName())
	assert.Contains(t, GetXRayTraceID(), "Root=1-xyz")
	assert.True(t, IsRunningInLambda())
	assert.True(t, IsXRayEnabled())
	assert.True(t, IsColdStart())
}
