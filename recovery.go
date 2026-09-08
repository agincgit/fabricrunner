package fabricrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrUncertainOutcome = errors.New("execution outcome is uncertain; reconciliation required")

// Resume continues only a committed turn boundary. It never repeats an
// unresolved model or tool call. The original request and IDs must be retained.
func (e *Engine) Resume(ctx context.Context, request EngineRequest) (*WorkloadProjection, error) {
	return e.run(ctx, request, true)
}

func requestDigest(request EngineRequest) (string, error) {
	definitions := make([]ToolDefinition, 0, len(request.Tools))
	for _, tool := range request.Tools {
		definitions = append(definitions, tool.Definition)
	}
	request.Tools = nil
	data, err := json.Marshal(struct {
		Request EngineRequest
		Tools   []ToolDefinition
	}{request, definitions})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
func (s *engineRun) restore(ctx context.Context, digest string) (*LoopCheckpoint, time.Time, error) {
	events, err := s.engine.Store.Load(ctx, s.aggregate, 0)
	if err != nil {
		return nil, time.Time{}, err
	}
	projection, err := NewWorkloadProjection(s.aggregate)
	if err != nil {
		return nil, time.Time{}, err
	}
	for _, event := range events {
		if err := projection.Apply(event); err != nil {
			return nil, time.Time{}, err
		}
	}
	if len(events) == 0 {
		return nil, time.Time{}, ErrAggregateNotFound
	}
	if projection.Workload.State != WorkloadRunning {
		return nil, time.Time{}, errors.New("only running workloads can resume")
	}
	var checkpoint *LoopEvent
	match := false
	for _, record := range projection.Execution {
		if record.Stage == ExecutionRequest {
			match = record.RequestSHA256 == digest
		}
		if record.Budget != nil {
			// A reservation is a durable effect intent, including compaction calls
			// that do not emit ordinary loop turn-start events.
			if record.Budget.Dimension == "" {
				checkpoint = nil
			}
			s.reserved.InputTokens += record.Budget.Reserved.InputTokens
			s.reserved.OutputTokens += record.Budget.Reserved.OutputTokens
			s.reserved.Cost += record.Budget.Reserved.Cost
		}
		if record.Compaction != nil {
			s.extraCalls++
			s.compactionUsage.InputTokens += record.Compaction.Usage.InputTokens
			s.compactionUsage.OutputTokens += record.Compaction.Usage.OutputTokens
			s.compactionUsage.CacheReadTokens += record.Compaction.Usage.CacheReadTokens
			s.compactionUsage.CacheWriteTokens += record.Compaction.Usage.CacheWriteTokens
			s.compactionUsage.Cost += record.Compaction.Usage.Cost
		}
		if record.Loop != nil {
			if record.Loop.Type == LoopEventCheckpoint {
				event := record.Loop.Clone()
				checkpoint = &event
			} else {
				checkpoint = nil
			}
		}
		// A compaction after a checkpoint is already a new external call. Do not
		// repeat it from an older boundary even if no normal turn has started yet.
		if record.Compaction != nil {
			checkpoint = nil
		}
	}
	if !match {
		return nil, time.Time{}, errors.New("resume request differs from committed request")
	}
	if checkpoint == nil {
		return nil, time.Time{}, ErrUncertainOutcome
	}
	if events[len(events)-1].AttemptID != s.request.AttemptID {
		return nil, time.Time{}, errors.New("resume attempt mismatch")
	}
	s.sequence = events[len(events)-1].Sequence
	s.stepApproved = true
	s.cause = events[len(events)-1].ID
	if err := s.record(ctx, ExecutionRecord{Stage: ExecutionResumed, RequestSHA256: digest}); err != nil {
		return nil, time.Time{}, err
	}
	return &LoopCheckpoint{Sequence: checkpoint.Sequence, Result: checkpoint.Checkpoint.Clone()}, projection.Workload.CreatedAt.Add(s.request.Budget.MaxWallTime), nil
}
