# Trace Context Propagation for Workflow Activities

## Overview

This document describes the changes made to durabletask-go to enable trace context propagation from Dapr consumer spans to Java workflow activity execution.

## Problem Statement

When Dapr executes workflow activities, it creates consumer spans for each activity. However, the trace context from these consumer spans was not being propagated to the Java client (durabletask-java), which meant:

1. HTTP calls made within Java activities were not appearing in distributed traces
2. The trace continuity was broken between Dapr's consumer span and the activity execution

## Solution

Extract the trace context from Dapr's consumer span and embed it in the `ActivityRequest` protobuf message using the W3C Trace Context format.

## Changes Made

### File: `backend/executor.go`

#### 1. Added Trace Context Extraction Function

Added `extractTraceContextFromCtx()` method (lines 275-303) to extract the active span context and convert it to W3C Trace Context format:

```go
// extractTraceContextFromCtx extracts the trace context from the context and returns a TraceContext protobuf message
// This is used to propagate trace context through the ActivityRequest protobuf
func (g *grpcExecutor) extractTraceContextFromCtx(ctx context.Context) *protos.TraceContext {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.IsValid() {
		g.logger.Debug("No valid span context found in context")
		return nil
	}

	// Build W3C Trace Context traceparent header: version-traceId-spanId-traceFlags
	traceparent := fmt.Sprintf("00-%s-%s-%02x",
		spanCtx.TraceID().String(),
		spanCtx.SpanID().String(),
		spanCtx.TraceFlags())

	traceCtx := &protos.TraceContext{
		TraceParent: traceparent,
	}

	// Add tracestate if present
	if stateStr := spanCtx.TraceState().String(); stateStr != "" {
		traceCtx.TraceState = wrapperspb.String(stateStr)
	}

	g.logger.Infof("Extracted trace context from ctx: traceID=%s spanID=%s flags=%02x",
		spanCtx.TraceID(), spanCtx.SpanID(), spanCtx.TraceFlags())

	return traceCtx
}
```

**Key Points:**
- Extracts `SpanContext` from the incoming `ctx` parameter
- The `ctx` already contains Dapr's consumer span (set by `executeActivity` in Dapr)
- Formats trace context as W3C Trace Context: `00-{traceId}-{spanId}-{traceFlags}`
- Includes optional `tracestate` if present

#### 2. Modified ExecuteActivity Function

Updated `ExecuteActivity()` method (lines 171-188) to inject trace context into `ActivityRequest`:

```go
task := e.GetTaskScheduled()

// Extract trace context from the incoming context to propagate to Java client
// If task.ParentTraceContext is not set, extract it from the context
parentTraceCtx := task.ParentTraceContext
if parentTraceCtx == nil {
	parentTraceCtx = executor.extractTraceContextFromCtx(ctx)
}

req := &protos.ActivityRequest{
	Name:                  task.Name,
	Version:               task.Version,
	Input:                 task.Input,
	OrchestrationInstance: &protos.OrchestrationInstance{InstanceId: string(iid)},
	TaskId:                e.EventId,
	TaskExecutionId:       task.TaskExecutionId,
	ParentTraceContext:    parentTraceCtx,  // ← Trace context embedded here
}
```

**Key Points:**
- Checks if `ParentTraceContext` is already set (for future use cases)
- Falls back to extracting from `ctx` if not set
- Embeds trace context in the `ActivityRequest` protobuf

## Architecture

```
Dapr Runtime (Go)
  └─ executeActivity() creates consumer span
       └─ calls durabletask-go ExecuteActivity(ctx)
            └─ ctx contains consumer span context
                 └─ extractTraceContextFromCtx(ctx)
                      └─ Embeds in ActivityRequest.ParentTraceContext
                           └─ Sent to Java client via GetWorkItems stream
```

## Why This Approach Works

1. **GetWorkItems is a Streaming RPC**: The stream is established once and kept open. Work items are queued and sent through the stream when activities are executed. There's no per-work-item gRPC call, so we can't use standard gRPC metadata propagation.

2. **Protobuf-Based Propagation**: The `ActivityRequest` protobuf already has a `TraceContext` field, making it the natural place for trace context propagation.

3. **Context Already Contains Span**: The `ctx` parameter passed to `ExecuteActivity()` already contains Dapr's consumer span, so we just need to extract and serialize it.

## Testing

To verify trace context extraction is working:

```bash
kubectl logs -l app=pizza-store-service -c daprd | grep "Extracted trace context"
```

Expected output:
```
Extracted trace context from ctx: traceID=c57c8abc4c298ffd0b9f28e3c7f61a66 spanID=b849bd4a0181444d flags=01
```

## Next Steps

The Java client (durabletask-java) needs to:
1. Extract the trace context from `ActivityRequest.ParentTraceContext`
2. Parse the W3C Trace Context format
3. Make the context current during activity execution
4. This enables the OpenTelemetry Java agent to automatically instrument HTTP calls

## References

- W3C Trace Context: https://www.w3.org/TR/trace-context/
- OpenTelemetry Go SDK: https://pkg.go.dev/go.opentelemetry.io/otel/trace
- durabletask-protobuf: https://github.com/dapr/durabletask-protobuf
