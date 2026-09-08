package fabricrunner_test

import (
	"encoding/json"
	fr "github.com/agincgit/fabricrunner"
	"reflect"
	"testing"
)

func TestExecutionProjectionRejectsMalformedRecordsWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*fr.ExecutionRecord)
	}{
		{"unknown_stage", func(r *fr.ExecutionRecord) { r.Stage = "unknown" }},
		{"negative_turn", func(r *fr.ExecutionRecord) { r.Turn = -1 }},
		{"sequence_gap", func(r *fr.ExecutionRecord) { r.Loop.Sequence++ }},
		{"wrong_placement", func(r *fr.ExecutionRecord) { r.Loop.Model.Model = "wrong" }},
		{"missing_payload", func(r *fr.ExecutionRecord) { r.Loop = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, r, _ := engineFixture(t)
			runEngine(t, e, r)
			events := engineEvents(t, e, r)
			p, err := fr.NewWorkloadProjection(fr.AggregateRef{Type: fr.WorkloadAggregateType, ID: r.WorkloadID})
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				var record fr.ExecutionRecord
				if event.Type == fr.EventTypeExecutionRecorded {
					if err := json.Unmarshal(event.Payload, &record); err != nil {
						t.Fatal(err)
					}
				}
				if record.Loop != nil && record.Loop.Type == fr.LoopEventTurnStarted {
					before, _ := json.Marshal(p)
					test.change(&record)
					d, err := fr.NewCoreEventDraft(event.Type, record)
					if err != nil {
						t.Fatal(err)
					}
					d.AttemptID = event.AttemptID
					d.CorrelationID = event.CorrelationID
					invalid, err := fr.NewEvent(event.Aggregate, event.Sequence, d, event.OccurredAt)
					if err != nil {
						t.Fatal(err)
					}
					if err := p.Apply(invalid); err == nil {
						t.Fatal("accepted malformed execution record")
					}
					after, _ := json.Marshal(p)
					if !reflect.DeepEqual(before, after) {
						t.Fatal("failed apply mutated projection")
					}
					return
				}
				if err := p.Apply(event); err != nil {
					t.Fatal(err)
				}
			}
			t.Fatal("turn not found")
		})
	}
}
