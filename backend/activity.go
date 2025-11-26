package backend

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/dapr/durabletask-go/api"
	"github.com/dapr/durabletask-go/api/helpers"
	"github.com/dapr/durabletask-go/api/protos"
)

type activityProcessor struct {
	be       Backend
	executor ActivityExecutor
	logger   Logger
}

type ActivityExecutor interface {
	ExecuteActivity(context.Context, api.InstanceID, *protos.HistoryEvent) (*protos.HistoryEvent, error)
}

func NewActivityTaskWorker(be Backend, executor ActivityExecutor, logger Logger, opts ...NewTaskWorkerOptions) TaskWorker[*ActivityWorkItem] {
	processor := newActivityProcessor(be, executor, logger)
	return NewTaskWorker(processor, logger, opts...)
}

func newActivityProcessor(be Backend, executor ActivityExecutor, logger Logger) TaskProcessor[*ActivityWorkItem] {
	return &activityProcessor{
		be:       be,
		executor: executor,
		logger:   logger,
	}
}

// Name implements TaskProcessor
func (*activityProcessor) Name() string {
	return "activity-processor"
}

// NextWorkItem implements TaskDispatcher
func (ap *activityProcessor) NextWorkItem(ctx context.Context) (*ActivityWorkItem, error) {
	return ap.be.NextActivityWorkItem(ctx)
}

// ProcessWorkItem implements TaskDispatcher
func (p *activityProcessor) ProcessWorkItem(ctx context.Context, awi *ActivityWorkItem) error {
	ts := awi.NewEvent.GetTaskScheduled()
	if ts == nil {
		return fmt.Errorf("%v: invalid TaskScheduled event", awi.InstanceID)
	}

	// Log incoming trace context
	if ts.ParentTraceContext != nil {
		p.logger.Infof("%v/%s#%d: ACTIVITY received ParentTraceContext: traceparent=%s tracestate=%v",
			awi.InstanceID, ts.Name, awi.NewEvent.EventId,
			ts.ParentTraceContext.GetTraceParent(), ts.ParentTraceContext.GetTraceState())
	} else {
		p.logger.Warnf("%v/%s#%d: ACTIVITY has NO ParentTraceContext - activity will start new trace!",
			awi.InstanceID, ts.Name, awi.NewEvent.EventId)
	}

	// Create span as child of spanContext found in TaskScheduledEvent
	ctx, err := helpers.ContextFromTraceContext(ctx, ts.ParentTraceContext)
	if err != nil {
		p.logger.Warnf("%v/%s#%d: failed to parse activity trace context: %v",
			awi.InstanceID, ts.Name, awi.NewEvent.EventId, err)
		return fmt.Errorf("%v: failed to parse activity trace context: %w", awi.InstanceID, err)
	}
	var span trace.Span
	ctx, span = helpers.StartNewActivitySpan(ctx, ts.Name, ts.Version.GetValue(), string(awi.InstanceID), awi.NewEvent.EventId)

	// Log the activity span context
	if span != nil {
		spanCtx := span.SpanContext()
		p.logger.Infof("%v/%s#%d: ACTIVITY SPAN started: traceID=%s spanID=%s flags=%02x",
			awi.InstanceID, ts.Name, awi.NewEvent.EventId,
			spanCtx.TraceID(), spanCtx.SpanID(), spanCtx.TraceFlags())
		defer func() {
			if r := recover(); r != nil {
				span.SetStatus(codes.Error, fmt.Sprintf("%v", r))
			}
			span.End()
		}()
	}

	// Set the parent trace context to be the current span
	// The OTel instrumentation will automatically propagate the trace context
	// via gRPC metadata, so we don't need to manually pass traceparent
	newTraceCtx := helpers.TraceContextFromSpan(span)
	ts.ParentTraceContext = newTraceCtx
	p.logger.Infof("%v/%s#%d: ACTIVITY updated ParentTraceContext for executor: traceparent=%s",
		awi.InstanceID, ts.Name, awi.NewEvent.EventId,
		newTraceCtx.GetTraceParent())

	// Execute the activity and get its result
	result, err := p.executor.ExecuteActivity(ctx, awi.InstanceID, awi.NewEvent)
	if err != nil {
		if span != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return err
	}
	awi.Result = result
	return nil
}

// CompleteWorkItem implements TaskDispatcher
func (ap *activityProcessor) CompleteWorkItem(ctx context.Context, awi *ActivityWorkItem) error {
	if awi.Result == nil {
		return fmt.Errorf("can't complete work item '%s' with nil result", awi)
	}
	if awi.Result.GetTaskCompleted() == nil && awi.Result.GetTaskFailed() == nil {
		return fmt.Errorf("can't complete work item '%s', which isn't TaskCompleted or TaskFailed", awi)
	}

	return ap.be.CompleteActivityWorkItem(ctx, awi)
}

// AbandonWorkItem implements TaskDispatcher
func (ap *activityProcessor) AbandonWorkItem(ctx context.Context, awi *ActivityWorkItem) error {
	return ap.be.AbandonActivityWorkItem(ctx, awi)
}
