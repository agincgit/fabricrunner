package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agincgit/fabricrunner"
	turnloop "github.com/agincgit/fabricrunner/loop"
	"github.com/agincgit/fabricrunner/providertest"
)

func TestProviderConformance(t *testing.T) {
	t.Parallel()
	providertest.Run(t, func(t *testing.T) providertest.Fixture {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer test-token" || request.Header.Get("anthropic-version") != defaultAPIVersion {
				t.Errorf("authentication headers = %#v", request.Header)
			}
			switch request.URL.Path {
			case "/v1/models":
				_, _ = io.WriteString(response, `{"data":[{"id":"model","max_input_tokens":4096,"max_tokens":1024}],"has_more":false}`)
			case "/v1/messages":
				writeEvents(response,
					namedEvent{"message_start", `{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":0}}}`},
					namedEvent{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
					namedEvent{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"done"}}`},
					namedEvent{"content_block_stop", `{"type":"content_block_stop","index":0}`},
					namedEvent{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":2,"output_tokens":1}}`},
					namedEvent{"message_stop", `{"type":"message_stop"}`},
				)
			default:
				http.NotFound(response, request)
			}
		}))
		t.Cleanup(server.Close)
		transport := &closeCountingTransport{base: http.DefaultTransport}
		capabilities := fabricrunner.ModelCapabilities{ContextTokens: 1, MaxOutputTokens: 1, Modalities: []string{"text"}, ToolUse: fabricrunner.ToolUseNative, Streaming: true}
		provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL + "/v1", AllowInsecureHTTP: true,
			TokenSource: func(context.Context) (string, error) { return "test-token", nil }, HTTPClient: &http.Client{Transport: transport},
			DefaultCapabilities: capabilities, Labels: map[string]string{"zone": "hosted"}})
		capabilities.ContextTokens, capabilities.MaxOutputTokens = 4096, 1024
		return providertest.Fixture{Provider: provider, Request: baseRequest(), WantName: "anthropic",
			WantModels: []fabricrunner.ModelDescriptor{{Ref: fabricrunner.ModelRef{Provider: "anthropic", Model: "model"}, Capabilities: capabilities, Labels: map[string]string{"zone": "hosted"}}},
			WantEvents: []fabricrunner.ModelEvent{
				{Type: fabricrunner.ModelEventStart, Sequence: 1},
				{Type: fabricrunner.ModelEventUsage, Sequence: 2, Usage: &fabricrunner.Usage{InputTokens: 2}},
				{Type: fabricrunner.ModelEventTextDelta, Sequence: 3, Text: "done"},
				{Type: fabricrunner.ModelEventUsage, Sequence: 4, Usage: &fabricrunner.Usage{OutputTokens: 1}},
				{Type: fabricrunner.ModelEventStop, Sequence: 5, Stop: fabricrunner.StopEndTurn},
			}, StreamCloseCount: func() int { return int(transport.closeCount.Load()) }}
	})
}

func TestConfigurationAuthenticationAndIsolation(t *testing.T) {
	t.Parallel()
	bad := []Config{{}, {Name: " bad", BaseURL: "https://example.com/v1"}, {Name: "ok", BaseURL: "http://example.com"},
		{Name: "ok", BaseURL: "https://user@example.com"}, {Name: "ok", BaseURL: "https://example.com?q=x"},
		{Name: "ok", BaseURL: "https://example.com", AuthMode: "bad"}, {Name: "ok", BaseURL: "https://example.com", APIVersion: "bad\nvalue"},
		{Name: "ok", BaseURL: "https://example.com", DefaultOutputTokens: -1}, {Name: "ok", BaseURL: "https://example.com", MaxModelPages: -1}}
	for index, config := range bad {
		if _, err := New(config); !errors.Is(err, ErrConfiguration) {
			t.Fatalf("config %d: %v", index, err)
		}
	}
	modalities := []string{"text"}
	labels := map[string]string{"zone": "one"}
	provider := mustProvider(t, Config{Name: "ok", BaseURL: "https://example.com", DefaultCapabilities: fabricrunner.ModelCapabilities{Modalities: modalities}, Labels: labels})
	modalities[0], labels["zone"] = "changed", "changed"
	if provider.defaultCapabilities.Modalities[0] != "text" || provider.labels["zone"] != "one" {
		t.Fatal("configuration aliases caller data")
	}

	var gotHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotHeader = request.Header.Clone()
		_, _ = io.WriteString(response, `{"data":[]}`)
	}))
	defer server.Close()
	provider = mustProvider(t, Config{Name: "ok", BaseURL: server.URL, AllowInsecureHTTP: true, AuthMode: AuthAPIKey, TokenSource: func(context.Context) (string, error) { return "secret", nil }})
	if _, err := provider.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotHeader.Get("x-api-key") != "secret" || gotHeader.Get("Authorization") != "" {
		t.Fatalf("authentication headers = %#v", gotHeader)
	}

	cause := errors.New("secret leaked")
	provider = mustProvider(t, Config{Name: "ok", BaseURL: "https://example.com", TokenSource: func(context.Context) (string, error) { return "", cause }})
	_, err := provider.Models(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret") || errors.Is(err, cause) {
		t.Fatalf("credential error = %v", err)
	}
}

func TestPaginatedModelDiscovery(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch calls.Add(1) {
		case 1:
			if request.URL.Query().Get("limit") != "1000" || request.URL.Query().Get("after_id") != "" {
				t.Errorf("first query = %s", request.URL.RawQuery)
			}
			_, _ = io.WriteString(response, `{"data":[{"id":"one","max_input_tokens":100,"max_tokens":20}],"has_more":true,"last_id":"cursor"}`)
		case 2:
			if request.URL.Query().Get("after_id") != "cursor" {
				t.Errorf("second query = %s", request.URL.RawQuery)
			}
			_, _ = io.WriteString(response, `{"data":[{"id":"two","max_input_tokens":200,"max_tokens":30}],"has_more":false}`)
		}
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
	models, err := provider.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].Ref.Model != "one" || models[1].Capabilities.ContextTokens != 200 || models[1].Capabilities.MaxOutputTokens != 30 {
		t.Fatalf("models = %#v", models)
	}
}

func TestModelDiscoveryFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, body string }{
		{"duplicate", `{"data":[{"id":"one"},{"id":"one"}]}`},
		{"empty ID", `{"data":[{"id":""}]}`},
		{"malformed", `{`},
		{"trailing", `{"data":[]} {}`},
		{"missing cursor", `{"data":[],"has_more":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(response, test.body) }))
			defer server.Close()
			provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
			if _, err := provider.Models(context.Background()); err == nil {
				t.Fatal("discovery unexpectedly succeeded")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(response, "secret response")
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
	_, err := provider.Models(context.Background())
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusTooManyRequests || strings.Contains(err.Error(), "secret") {
		t.Fatalf("HTTP error = %v", err)
	}
}

func TestExactRequestTranslation(t *testing.T) {
	t.Parallel()
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		writeEndTurn(response)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
	request := baseRequest()
	request.Messages = []fabricrunner.Message{
		{Role: fabricrunner.RoleSystem, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "rules"}}},
		{Role: fabricrunner.RoleAssistant, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "calling"}, {Type: fabricrunner.ContentToolCall, Classification: fabricrunner.ClassInternal, ToolCallID: "call-1", ToolName: "lookup", JSON: json.RawMessage(`{"q":"x"}`)}}},
		{Role: fabricrunner.RoleTool, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentToolResult, Classification: fabricrunner.ClassInternal, ToolCallID: "call-1", JSON: json.RawMessage(`{"value":1}`), IsError: true}}},
		{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "continue"}}},
	}
	request.Tools = []fabricrunner.ToolDefinition{{Name: "lookup", Description: "Lookup", InputSchema: json.RawMessage(`{"type":"object"}`), Strict: true}}
	request.ToolChoice = fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceNamed, Name: "lookup"}
	request.Output = &fabricrunner.OutputConstraint{Name: "answer", Schema: json.RawMessage(`{"type":"object"}`), Strict: true}
	request.MaxOutputTokens = 321
	stream, err := provider.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fabricrunner.DrainStream(context.Background(), stream); err != nil {
		t.Fatal(err)
	}
	if body["model"] != "model" || body["max_tokens"] != float64(321) || body["stream"] != true {
		t.Fatalf("controls = %#v", body)
	}
	if body["system"].([]any)[0].(map[string]any)["text"] != "rules" {
		t.Fatalf("system = %#v", body["system"])
	}
	messages := body["messages"].([]any)
	toolResult := messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "call-1" || toolResult["is_error"] != true {
		t.Fatalf("tool result = %#v", toolResult)
	}
	choice := body["tool_choice"].(map[string]any)
	if choice["type"] != "tool" || choice["name"] != "lookup" {
		t.Fatalf("choice = %#v", choice)
	}
	format := body["output_config"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" {
		t.Fatalf("format = %#v", format)
	}
}

func TestToolChoiceMappingsAndLocalRejection(t *testing.T) {
	t.Parallel()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: "https://example.com/v1"})
	tool := fabricrunner.ToolDefinition{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}
	tests := []struct {
		choice fabricrunner.ToolChoice
		kind   string
		name   string
	}{
		{fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceAuto}, "auto", ""},
		{fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceNone}, "none", ""},
		{fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceRequired}, "any", ""},
		{fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceNamed, Name: "lookup"}, "tool", "lookup"},
	}
	for _, test := range tests {
		request := baseRequest()
		request.Tools, request.ToolChoice = []fabricrunner.ToolDefinition{tool}, test.choice
		wire, err := provider.translateRequest(request)
		if err != nil || wire.ToolChoice.Type != test.kind || wire.ToolChoice.Name != test.name {
			t.Fatalf("choice %#v => %#v, %v", test.choice, wire.ToolChoice, err)
		}
	}
	request := baseRequest()
	request.ToolChoice.Mode = fabricrunner.ToolChoiceRequired
	if _, err := provider.translateRequest(request); !errors.Is(err, ErrTranslation) {
		t.Fatalf("required without tools error = %v", err)
	}
	request = baseRequest()
	request.Messages = append(request.Messages, fabricrunner.Message{Role: fabricrunner.RoleSystem, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "late"}}})
	if _, err := provider.translateRequest(request); !errors.Is(err, ErrTranslation) {
		t.Fatalf("late system error = %v", err)
	}
}

func TestToolStreamAndCumulativeUsage(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeEvents(response,
			namedEvent{"message_start", `{"type":"message_start","message":{"usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":3,"cache_creation_input_tokens":2}}}`},
			namedEvent{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call-1","name":"lookup","input":{}}}`},
			namedEvent{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`},
			namedEvent{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"go\"}"}}`},
			namedEvent{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			namedEvent{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`},
			namedEvent{"message_stop", `{"type":"message_stop"}`},
		)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
	stream, err := provider.Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	events, err := fabricrunner.DrainStream(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	var call *fabricrunner.ToolCall
	var totals fabricrunner.Usage
	for _, event := range events {
		if event.ToolCall != nil {
			call = event.ToolCall
		}
		if event.Usage != nil {
			totals.InputTokens += event.Usage.InputTokens
			totals.OutputTokens += event.Usage.OutputTokens
			totals.CacheReadTokens += event.Usage.CacheReadTokens
			totals.CacheWriteTokens += event.Usage.CacheWriteTokens
		}
	}
	if call == nil || string(call.Input) != `{"q":"go"}` || totals != (fabricrunner.Usage{InputTokens: 10, OutputTokens: 4, CacheReadTokens: 3, CacheWriteTokens: 2}) || events[len(events)-1].Stop != fabricrunner.StopToolUse {
		t.Fatalf("events = %#v, totals = %#v", events, totals)
	}
}

func TestProtocolFailuresAreTyped(t *testing.T) {
	t.Parallel()
	start := namedEvent{"message_start", `{"type":"message_start","message":{"usage":{}}}`}
	tests := []struct {
		name   string
		events []namedEvent
		limit  int
	}{
		{"before start", []namedEvent{{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`}}, 0},
		{"name mismatch", []namedEvent{{"wrong", `{"type":"message_start","message":{"usage":{}}}`}}, 0},
		{"unterminated", []namedEvent{start}, 0},
		{"negative usage", []namedEvent{{"message_start", `{"type":"message_start","message":{"usage":{"input_tokens":-1}}}`}}, 0},
		{"invalid tool JSON", []namedEvent{start,
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call-1","name":"lookup","input":{}}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`}}, 0},
		{"overlapping blocks", []namedEvent{start,
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`}}, 0},
		{"duplicate call ID", []namedEvent{start,
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"same","name":"one","input":{}}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"same","name":"two","input":{}}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":1}`}}, 0},
		{"decreasing usage", []namedEvent{{"message_start", `{"type":"message_start","message":{"usage":{"input_tokens":10}}}`},
			{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5}}`}}, 0},
		{"oversized", []namedEvent{start}, 16},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { writeEvents(response, test.events...) }))
			defer server.Close()
			provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true, MaxEventBytes: test.limit})
			stream, err := provider.Stream(context.Background(), baseRequest())
			if err != nil {
				t.Fatal(err)
			}
			_, err = fabricrunner.DrainStream(context.Background(), stream)
			if !errors.Is(err, ErrSSEProtocol) {
				t.Fatalf("events %#v error = %v", test.events, err)
			}
		})
	}
}

func TestInStreamErrorIsNormalizedAndRedacted(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		writeEvents(response, namedEvent{"error", `{"type":"error","error":{"type":"overloaded_error","message":"secret provider detail"}}`})
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
	stream, err := provider.Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	events, err := fabricrunner.DrainStream(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != fabricrunner.ModelEventStart || events[1].Type != fabricrunner.ModelEventError || strings.Contains(events[1].Error.Message, "secret") {
		t.Fatalf("events = %#v", events)
	}
}

func TestCancellationAndClose(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(200)
		response.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	transport := &closeCountingTransport{base: http.DefaultTransport}
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true, HTTPClient: &http.Client{Transport: transport}})
	stream, err := provider.Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := stream.Recv(ctx); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("receive did not cancel")
	}
	_ = stream.Close()
	_ = stream.Close()
	if transport.closeCount.Load() != 1 {
		t.Fatalf("close count = %d", transport.closeCount.Load())
	}
}

func TestProviderRunsInsideCanonicalToolLoop(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var secondRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			writeEvents(response,
				namedEvent{"message_start", `{"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":0}}}`},
				namedEvent{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call-1","name":"lookup","input":{}}}`},
				namedEvent{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"go\"}"}}`},
				namedEvent{"content_block_stop", `{"type":"content_block_stop","index":0}`},
				namedEvent{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":2}}`},
				namedEvent{"message_stop", `{"type":"message_stop"}`},
			)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&secondRequest); err != nil {
			t.Error(err)
		}
		writeEvents(response,
			namedEvent{"message_start", `{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":0}}}`},
			namedEvent{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			namedEvent{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"final"}}`},
			namedEvent{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			namedEvent{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`},
			namedEvent{"message_stop", `{"type":"message_stop"}`},
		)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "anthropic", BaseURL: server.URL, AllowInsecureHTTP: true})
	selector := fixedSelector{provider: provider, model: fabricrunner.ModelRef{Provider: "anthropic", Model: "model"}}
	tool := &adapterTool{}
	request := fabricrunner.LoopRequest{
		WorkloadID: newTestID(t), StepID: newTestID(t), AttemptID: newTestID(t),
		Initial:                   fabricrunner.ModelRequest{Model: selector.model, Messages: []fabricrunner.Message{{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "question"}}}}, ToolChoice: fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceAuto}, MaxOutputTokens: 100},
		Tools:                     []fabricrunner.ToolBinding{{Definition: fabricrunner.ToolDefinition{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","required":["query"]}`), OutputSchema: json.RawMessage(`{"type":"object","required":["value"]}`)}, Handler: tool}},
		Selector:                  selector,
		Budget:                    fabricrunner.Budget{MaxInputTokens: 1000, MaxOutputTokens: 1000, MaxModelCalls: 3, MaxToolCalls: 2, MaxCost: 1000, MaxWallTime: time.Second},
		ModelOutputClassification: fabricrunner.ClassInternal, ToolErrorClassification: fabricrunner.ClassInternal,
	}
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stop != fabricrunner.StopEndTurn || result.ModelCalls != 2 || result.ToolCalls != 1 || result.Messages[len(result.Messages)-1].Content[0].Text != "final" || tool.executeCount.Load() != 1 || tool.closeCount.Load() != 1 {
		t.Fatalf("loop result = %#v", result)
	}
	messages := secondRequest["messages"].([]any)
	toolMessage := messages[len(messages)-1].(map[string]any)
	toolResult := toolMessage["content"].([]any)[0].(map[string]any)
	if toolMessage["role"] != "user" || toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "call-1" || toolResult["content"] != `{"value":"result"}` {
		t.Fatalf("second-turn tool result = %#v", toolMessage)
	}
}

func baseRequest() fabricrunner.ModelRequest {
	return fabricrunner.ModelRequest{Model: fabricrunner.ModelRef{Provider: "anthropic", Model: "model"}, Messages: []fabricrunner.Message{{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "hello"}}}}, ToolChoice: fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceNone}}
}
func mustProvider(t *testing.T, config Config) *Provider {
	t.Helper()
	provider, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func newTestID(t *testing.T) fabricrunner.ID {
	t.Helper()
	id, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type fixedSelector struct {
	provider fabricrunner.Provider
	model    fabricrunner.ModelRef
}

func (selector fixedSelector) SelectTurn(context.Context, fabricrunner.TurnContext) (fabricrunner.TurnSelection, error) {
	return fabricrunner.TurnSelection{Provider: selector.provider, Model: selector.model}, nil
}

type adapterTool struct {
	executeCount atomic.Int32
	closeCount   atomic.Int32
}

func (tool *adapterTool) Execute(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
	tool.executeCount.Add(1)
	return fabricrunner.ToolOutput{JSON: json.RawMessage(`{"value":"result"}`), Classification: fabricrunner.ClassInternal}, nil
}

func (tool *adapterTool) Close(context.Context) error {
	tool.closeCount.Add(1)
	return nil
}

type namedEvent struct{ name, data string }

func writeEvents(response http.ResponseWriter, events ...namedEvent) {
	response.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		_, _ = fmt.Fprintf(response, "event: %s\ndata: %s\n\n", event.name, event.data)
	}
	response.(http.Flusher).Flush()
}
func writeEndTurn(response http.ResponseWriter) {
	writeEvents(response, namedEvent{"message_start", `{"type":"message_start","message":{"usage":{}}}`}, namedEvent{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{}}`}, namedEvent{"message_stop", `{"type":"message_stop"}`})
}

type closeCountingTransport struct {
	base       http.RoundTripper
	closeCount atomic.Int32
}

func (transport *closeCountingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	response.Body = &countingReadCloser{ReadCloser: response.Body, count: &transport.closeCount}
	return response, nil
}

type countingReadCloser struct {
	io.ReadCloser
	count *atomic.Int32
	once  sync.Once
}

func (body *countingReadCloser) Close() error {
	body.once.Do(func() { body.count.Add(1) })
	return body.ReadCloser.Close()
}
