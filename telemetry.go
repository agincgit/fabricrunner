package fabricrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

const RecordSchemaV1 = uint32(1)

type RecordKind string

const (
	RecordWorkload RecordKind = "workload"
	RecordStep     RecordKind = "step"
	RecordAttempt  RecordKind = "attempt"
	RecordRouting  RecordKind = "routing"
	RecordPolicy   RecordKind = "policy"
	RecordProvider RecordKind = "provider"
	RecordTool     RecordKind = "tool"
)

type AttributeKey string

const (
	AttrOutcome AttributeKey = "outcome"
	AttrPhase   AttributeKey = "phase"
)

// Record is operational metadata. Sequence orders records within one execution;
// exporters may assign receipt times. Durable audit comes from EventStore.
type Record struct {
	SchemaVersion  uint32
	Kind           RecordKind
	WorkloadID     ID
	StepID         ID
	AttemptID      ID
	Sequence       uint64
	Classification Classification
	Content        []byte
	ContentSize    int64
	ContentSHA256  string
	Attributes     map[AttributeKey]string
}
type RecordInput struct {
	Kind           RecordKind
	WorkloadID     ID
	StepID         ID
	AttemptID      ID
	Sequence       uint64
	Classification Classification
	Content        []byte
	Attributes     map[AttributeKey]string
}

func NewRecord(input RecordInput) (Record, error) {
	switch input.Kind {
	case RecordWorkload, RecordStep, RecordAttempt, RecordRouting, RecordPolicy, RecordProvider, RecordTool:
	default:
		return Record{}, errors.New("unknown observation kind")
	}
	if input.Classification == "" {
		input.Classification = ClassConfidential
	}
	if err := input.Classification.Validate(); err != nil {
		return Record{}, err
	}
	for key := range input.Attributes {
		switch key {
		case AttrOutcome, AttrPhase:
		default:
			return Record{}, errors.New("unknown observation attribute key")
		}
	}
	r := Record{SchemaVersion: RecordSchemaV1, Kind: input.Kind, WorkloadID: input.WorkloadID, StepID: input.StepID, AttemptID: input.AttemptID, Sequence: input.Sequence, Classification: input.Classification}
	if input.Content != nil {
		hash := sha256.Sum256(input.Content)
		r.ContentSize = int64(len(input.Content))
		r.ContentSHA256 = hex.EncodeToString(hash[:])
	}
	if classificationRank(input.Classification) < classificationRank(ClassConfidential) {
		r.Content = append([]byte(nil), input.Content...)
		if input.Attributes != nil {
			r.Attributes = make(map[AttributeKey]string, len(input.Attributes))
			for k, v := range input.Attributes {
				r.Attributes[k] = v
			}
		}
	}
	return r, nil
}

type Observer interface {
	Observe(context.Context, Record) error
}

// Observation bounds process-wide in-flight callbacks, including observers
// ignoring cancellation. Saturation drops operational records, never audit.
// Go cannot forcibly stop a callback; at most 64 stuck callbacks can remain.
type Observation struct {
	observer Observer
	timeout  time.Duration
}

var observationSlots = make(chan struct{}, 64)

func NewObservation(observer Observer, timeout time.Duration) *Observation {
	if observer == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	return &Observation{observer: observer, timeout: timeout}
}

// Emit never waits for an exporter. A nil receiver performs only a nil check.
func (o *Observation) Emit(ctx context.Context, r Record) {
	if o == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	// Reconstruct to protect the boundary even if a caller assembled Record directly.
	safe, err := NewRecord(RecordInput{Kind: r.Kind, WorkloadID: r.WorkloadID, StepID: r.StepID, AttemptID: r.AttemptID, Sequence: r.Sequence, Classification: r.Classification, Content: r.Content, Attributes: r.Attributes})
	if err != nil {
		return
	}
	if r.Content == nil && r.ContentSHA256 != "" {
		if validateSHA256("observation hash", r.ContentSHA256) != nil || r.ContentSize < 0 {
			return
		}
		safe.ContentSHA256 = r.ContentSHA256
		safe.ContentSize = r.ContentSize
	}
	select {
	case observationSlots <- struct{}{}:
	default:
		return
	}
	observeCtx, cancel := context.WithTimeout(ctx, o.timeout)
	go func() {
		defer func() { _ = recover(); cancel(); <-observationSlots }()
		if observeCtx.Err() == nil {
			_ = o.observer.Observe(observeCtx, safe)
		}
	}()
}
