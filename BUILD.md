# Build Instructions for durabletask-go

## Overview

This document provides step-by-step instructions for building durabletask-go with trace context propagation changes.

## Prerequisites

- Go 1.21 or later
- Git

## Build Steps

### 1. Navigate to the Repository

```bash
cd /Users/kaspernissen/dash0/dapr/newnewclone/durabletask-go
```

### 2. Verify Changes

Check that the trace context extraction changes are present:

```bash
git status
```

Expected modified files:
- `backend/executor.go`

### 3. Build the Project

```bash
go build ./...
```

### 4. Run Tests

```bash
go test ./...
```

### 5. Install Locally (Optional)

If you want to use this version in other Go projects locally:

```bash
go install ./...
```

## Using in Dapr

Since Dapr uses durabletask-go as a dependency, you need to configure Dapr to use your local version:

### Option 1: Replace Directive in go.mod

In the Dapr repository's `go.mod`, add a replace directive:

```go
replace github.com/dapr/durabletask-go => /Users/kaspernissen/dash0/dapr/newnewclone/durabletask-go
```

### Option 2: Publish to Fork

1. Fork the durabletask-go repository
2. Push your changes to a branch
3. Update Dapr's go.mod to use your fork:

```go
require github.com/YOUR_USERNAME/durabletask-go v0.0.0-20250122000000-abcdef123456
```

## Verification

After building, verify the trace context extraction function exists:

```bash
grep -n "extractTraceContextFromCtx" backend/executor.go
```

Expected output should show the function at line ~275.

## Changes Summary

The build includes:

1. **New Function**: `extractTraceContextFromCtx()` - Extracts trace context from Go context and formats as W3C Trace Context
2. **Modified Function**: `ExecuteActivity()` - Embeds trace context in ActivityRequest protobuf

## Dependencies

This version adds no new dependencies. It uses existing OpenTelemetry packages already in go.mod:
- `go.opentelemetry.io/otel/trace`
- `google.golang.org/protobuf`

## Build Output

Successful build will show no errors. Example output:

```
$ go build ./...
$
```

## Troubleshooting

### Error: "cannot find package"

Make sure you're in the correct directory:
```bash
pwd
# Should output: /Users/kaspernissen/dash0/dapr/newnewclone/durabletask-go
```

### Error: "undefined: trace"

Make sure the imports are present in `backend/executor.go`:
```go
import (
    "go.opentelemetry.io/otel/trace"
    // ... other imports
)
```

## Next Steps

After building durabletask-go:

1. Build Dapr with the updated durabletask-go dependency (see `/Users/kaspernissen/dash0/dapr/newnewclone/dapr/BUILD.md`)
2. Build durabletask-java to extract the trace context (see `/Users/kaspernissen/dash0/dapr/newnewclone/durabletask-java/BUILD.md`)

## Version Information

- Version: Based on latest main branch
- Commit: Check `git rev-parse HEAD` for current commit hash

## Related Documentation

- `TRACE_CONTEXT_PROPAGATION.md` - Technical details of the implementation
- Dapr `WORKFLOW_ACTIVITY_TRACING.md` - How Dapr uses this functionality
- durabletask-java `TRACE_CONTEXT_PROPAGATION.md` - Client-side implementation
