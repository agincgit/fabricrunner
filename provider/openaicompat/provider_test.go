package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
			if request.Header.Get("Authorization") != "Bearer test-token" {
				t.Errorf("authorization = %q", request.Header.Get("Authorization"))
			}
			switch request.URL.Path {
			case "/v1/models":
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, `{"data":[{"id":"model"}]}`)
			case "/v1/chat/completions":
				writeSSE(response,
					`{"id":"resp-1","model":"model","system_fingerprint":"fp-1","choices":[{"index":0,"delta":{"content":"done"}}]}`,
					`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`,
				)
			default:
				http.NotFound(response, request)
			}
		}))
		t.Cleanup(server.Close)
		transport := &closeCountingTransport{base: http.DefaultTransport}
		provider := mustProvider(t, Config{
			Name: "compatible", BaseURL: server.URL + "/v1", AllowInsecureHTTP: true,
			TokenSource: func(context.Context) (string, error) { return "test-token", nil },
			HTTPClient:  &http.Client{Transport: transport},
			DefaultCapabilities: fabricrunner.ModelCapabilities{
				ContextTokens: 4096, MaxOutputTokens: 1024, Modalities: []string{"text"},
				ToolUse: fabricrunner.ToolUseNative, Streaming: true,
			},
			Labels: map[string]string{"zone": "test"},
		})
		models := []fabricrunner.ModelDescriptor{{
			Ref: fabricrunner.ModelRef{Provider: "compatible", Model: "model"},
			Capabilities: fabricrunner.ModelCapabilities{
				ContextTokens: 4096, MaxOutputTokens: 1024, Modalities: []string{"text"},
				ToolUse: fabricrunner.ToolUseNative, Streaming: true,
			},
			Labels: map[string]string{"zone": "test"},
		}}
		return providertest.Fixture{
			Provider: provider, Request: baseRequest("compatible"), WantName: "compatible", WantModels: models,
			WantEvents: []fabricrunner.ModelEvent{
				{Type: fabricrunner.ModelEventStart, Sequence: 1},
				{Type: fabricrunner.ModelEventTextDelta, Sequence: 2, Text: "done"},
				{Type: fabricrunner.ModelEventUsage, Sequence: 3, Usage: &fabricrunner.Usage{InputTokens: 2, OutputTokens: 1}},
				{Type: fabricrunner.ModelEventStop, Sequence: 4, Stop: fabricrunner.StopEndTurn},
			},
			StreamCloseCount: func() int { return int(transport.closeCount.Load()) },
		}
	})
}

func TestConfigurationValidationAndIsolation(t *testing.T) {
	t.Parallel()

	tests := []Config{
		{},
		{Name: " test", BaseURL: "https://example.com/v1"},
		{Name: "test", BaseURL: " https://example.com/v1"},
		{Name: "test", BaseURL: "relative"},
		{Name: "test", BaseURL: "http://example.com/v1"},
		{Name: "test", BaseURL: "ftp://example.com/v1"},
		{Name: "test", BaseURL: "https://user@example.com/v1"},
		{Name: "test", BaseURL: "https://example.com/v1?q=1"},
		{Name: "test", BaseURL: "https://example.com/v1#fragment"},
		{Name: "test", BaseURL: "https://example.com/v1", TokenLimitField: "both"},
		{Name: "test", BaseURL: "https://example.com/v1", MaxBodyBytes: -1},
		{Name: "test", BaseURL: "https://example.com/v1", MaxEventBytes: -1},
	}
	for index, config := range tests {
		if _, err := New(config); !errors.Is(err, ErrConfiguration) {
			t.Fatalf("config %d error = %v, want ErrConfiguration", index, err)
		}
	}

	modalities := []string{"text"}
	labels := map[string]string{"zone": "personal"}
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: "https://example.com/v1",
		DefaultCapabilities: fabricrunner.ModelCapabilities{Modalities: modalities, ToolUse: fabricrunner.ToolUseNone},
		Labels:              labels,
	})
	modalities[0] = "mutated"
	labels["zone"] = "mutated"
	if provider.defaultCapabilities.Modalities[0] != "text" || provider.labels["zone"] != "personal" {
		t.Fatal("configuration aliases caller-owned data")
	}
}

func TestModelDiscoveryValidationAndCredentialRedaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		body       string
		maxBody    int64
		wantHTTP   bool
		wantSecret bool
	}{
		{name: "duplicate", status: 200, body: `{"data":[{"id":"one"},{"id":"one"}]}`},
		{name: "empty ID", status: 200, body: `{"data":[{"id":""}]}`},
		{name: "malformed", status: 200, body: `{`},
		{name: "trailing", status: 200, body: `{"data":[]} {}`},
		{name: "oversized", status: 200, body: `{"data":[]}`, maxBody: 2},
		{name: "HTTP error", status: 401, body: `super-secret`, wantHTTP: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			provider := mustProvider(t, Config{
				Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true,
				TokenSource:  func(context.Context) (string, error) { return "super-secret", nil },
				MaxBodyBytes: test.maxBody,
			})
			_, err := provider.Models(context.Background())
			if err == nil || strings.Contains(err.Error(), "super-secret") {
				t.Fatalf("error = %v", err)
			}
			var httpError *HTTPError
			if errors.As(err, &httpError) != test.wantHTTP {
				t.Fatalf("HTTP error = %v, want %v", errors.As(err, &httpError), test.wantHTTP)
			}
		})
	}

	secretCause := errors.New("credential super-secret failed")
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: "https://example.com",
		TokenSource: func(context.Context) (string, error) { return "", secretCause },
	})
	_, err := provider.Models(context.Background())
	if err == nil || strings.Contains(err.Error(), "super-secret") || errors.Is(err, secretCause) {
		t.Fatalf("credential error = %v", err)
	}
}

func TestExactRequestTranslationAndUnsupportedContent(t *testing.T) {
	t.Parallel()

	var requestBody []byte
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		requestBody, _ = io.ReadAll(request.Body)
		writeSSE(response, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true,
		TokenLimitField: TokenLimitMaxCompletionTokens,
	})
	request := baseRequest("test")
	request.Messages = []fabricrunner.Message{
		{Role: fabricrunner.RoleSystem, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "system"}}},
		{Role: fabricrunner.RoleAssistant, Content: []fabricrunner.ContentPart{
			{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "calling"},
			{Type: fabricrunner.ContentToolCall, Classification: fabricrunner.ClassInternal, ToolCallID: "call-1", ToolName: "lookup", JSON: json.RawMessage(`{"query":"one"}`)},
		}},
		{Role: fabricrunner.RoleTool, Content: []fabricrunner.ContentPart{
			{Type: fabricrunner.ContentToolResult, Classification: fabricrunner.ClassInternal, ToolCallID: "call-1", ToolName: "lookup", JSON: json.RawMessage(`{"value":"one"}`)},
			{Type: fabricrunner.ContentToolResult, Classification: fabricrunner.ClassInternal, ToolCallID: "call-2", ToolName: "lookup", JSON: json.RawMessage(`{"value":"two"}`)},
		}},
		{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "continue"}}},
	}
	request.Tools = []fabricrunner.ToolDefinition{{
		Name: "lookup", Description: "Look up a value", InputSchema: json.RawMessage(`{"type":"object"}`), Strict: true,
	}}
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
	var got map[string]any
	if err := json.Unmarshal(requestBody, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "model" || got["stream"] != true || got["max_completion_tokens"] != float64(321) || got["max_tokens"] != nil {
		t.Fatalf("request controls = %#v", got)
	}
	messages := got["messages"].([]any)
	if len(messages) != 5 || messages[2].(map[string]any)["tool_call_id"] != "call-1" ||
		messages[3].(map[string]any)["tool_call_id"] != "call-2" {
		t.Fatalf("wire messages = %#v", messages)
	}
	choice := got["tool_choice"].(map[string]any)
	if choice["type"] != "function" || choice["function"].(map[string]any)["name"] != "lookup" {
		t.Fatalf("tool choice = %#v", choice)
	}
	format := got["response_format"].(map[string]any)
	if format["type"] != "json_schema" || format["json_schema"].(map[string]any)["name"] != "answer" {
		t.Fatalf("response format = %#v", format)
	}
	tool := got["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "lookup" || tool["strict"] != true ||
		tool["parameters"].(map[string]any)["type"] != "object" ||
		got["stream_options"].(map[string]any)["include_usage"] != true {
		t.Fatalf("tool/stream options = %#v / %#v", tool, got["stream_options"])
	}

	invalid := baseRequest("test")
	artifactID, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	invalid.Messages[0].Content = []fabricrunner.ContentPart{{
		Type: fabricrunner.ContentImage, Classification: fabricrunner.ClassInternal, ArtifactID: artifactID,
	}}
	if _, err := provider.Stream(context.Background(), invalid); !errors.Is(err, ErrTranslation) {
		t.Fatalf("unsupported content error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("HTTP requests = %d, want 1", requests.Load())
	}
}

func TestToolChoiceAndTokenFieldMappings(t *testing.T) {
	t.Parallel()

	choices := []fabricrunner.ToolChoice{
		{Mode: fabricrunner.ToolChoiceAuto},
		{Mode: fabricrunner.ToolChoiceNone},
		{Mode: fabricrunner.ToolChoiceRequired},
		{Mode: fabricrunner.ToolChoiceNamed, Name: "lookup"},
	}
	for _, field := range []TokenLimitField{TokenLimitMaxTokens, TokenLimitMaxCompletionTokens} {
		for _, choice := range choices {
			provider := mustProvider(t, Config{
				Name: "test", BaseURL: "https://example.com/v1", TokenLimitField: field,
			})
			request := baseRequest("test")
			request.Tools = []fabricrunner.ToolDefinition{{Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}
			request.ToolChoice = choice
			request.MaxOutputTokens = 99
			translated, err := provider.translateRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			if field == TokenLimitMaxTokens && (translated.MaxTokens == nil || *translated.MaxTokens != 99 || translated.MaxCompletionTokens != nil) {
				t.Fatalf("max_tokens mapping = %#v", translated)
			}
			if field == TokenLimitMaxCompletionTokens && (translated.MaxCompletionTokens == nil || *translated.MaxCompletionTokens != 99 || translated.MaxTokens != nil) {
				t.Fatalf("max_completion_tokens mapping = %#v", translated)
			}
			switch choice.Mode {
			case fabricrunner.ToolChoiceNamed:
				mapped, ok := translated.ToolChoice.(namedToolChoice)
				if !ok || mapped.Function.Name != choice.Name {
					t.Fatalf("named choice = %#v", translated.ToolChoice)
				}
			default:
				if translated.ToolChoice != string(choice.Mode) {
					t.Fatalf("choice = %#v, want %q", translated.ToolChoice, choice.Mode)
				}
			}
		}
	}
}

func TestCredentialsResolvePerRequestAndRedirectsAreRejected(t *testing.T) {
	t.Parallel()

	var credentials atomic.Int32
	var authorizationsMu sync.Mutex
	var authorizations []string
	var redirected atomic.Int32
	var redirectMode atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		authorizationsMu.Lock()
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		authorizationsMu.Unlock()
		if redirectMode.Load() {
			http.Redirect(response, request, target.URL, http.StatusFound)
			return
		}
		_, _ = io.WriteString(response, `{"data":[]}`)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true,
		TokenSource: func(context.Context) (string, error) {
			return fmt.Sprintf("token-%d", credentials.Add(1)), nil
		},
	})
	if _, err := provider.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Models(context.Background()); err != nil {
		t.Fatal(err)
	}
	if credentials.Load() != 2 || !reflect.DeepEqual(authorizations, []string{"Bearer token-1", "Bearer token-2"}) {
		t.Fatalf("credentials = %d, headers = %v", credentials.Load(), authorizations)
	}

	redirectMode.Store(true)
	_, err := provider.Models(context.Background())
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusFound {
		t.Fatalf("redirect error = %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatalf("redirect target calls = %d", redirected.Load())
	}
}

func TestDiscoveryCancellation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := provider.Models(ctx)
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("discovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop discovery")
	}
}

func TestPreCanceledContextHasNoCredentialOrTransportSideEffect(t *testing.T) {
	t.Parallel()

	var credentials atomic.Int32
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: "https://example.com/v1",
		TokenSource: func(context.Context) (string, error) {
			credentials.Add(1)
			return "token", nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Models(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("models error = %v", err)
	}
	if _, err := provider.Stream(ctx, baseRequest("test")); !errors.Is(err, context.Canceled) {
		t.Fatalf("stream error = %v", err)
	}
	if credentials.Load() != 0 {
		t.Fatalf("credential calls = %d, want 0", credentials.Load())
	}
}

func TestSSEToolCallsUsageAndOrdering(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		flusher := response.(http.Flusher)
		_, _ = io.WriteString(response, ": keepalive\n")
		_, _ = io.WriteString(response, "data: {\"id\":\"resp\",\n")
		_, _ = io.WriteString(response, "data: \"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"call-2\",\"type\":\"function\",\"function\":{\"name\":\"second\",\"arguments\":\"{\\\"b\\\":\"}},{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"first\",\"arguments\":\"{\\\"a\\\":\"}}]}}]}\n\n")
		_, _ = io.WriteString(response, "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"function\":{\"arguments\":\"2}\"}},{\"index\":0,\"function\":{\"arguments\":\"1}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\n")
		_, _ = io.WriteString(response, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true})
	stream, err := provider.Stream(context.Background(), baseRequest("test"))
	if err != nil {
		t.Fatal(err)
	}
	events, err := fabricrunner.DrainStream(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	var completed []fabricrunner.ToolCall
	var deltaIndices []int
	for _, event := range events {
		if event.ToolCallDelta != nil {
			deltaIndices = append(deltaIndices, event.ToolCallDelta.Index)
		}
		if event.ToolCall != nil {
			completed = append(completed, *event.ToolCall)
		}
	}
	if !reflect.DeepEqual(deltaIndices, []int{1, 0, 1, 0}) || len(completed) != 2 ||
		completed[0].ID != "call-1" || string(completed[0].Input) != `{"a":1}` ||
		completed[1].ID != "call-2" || string(completed[1].Input) != `{"b":2}` {
		t.Fatalf("deltas = %v, calls = %#v", deltaIndices, completed)
	}
	usage := events[len(events)-2]
	stop := events[len(events)-1]
	if usage.Type != fabricrunner.ModelEventUsage || usage.Usage.InputTokens != 5 ||
		usage.Usage.OutputTokens != 3 || usage.Usage.CacheReadTokens != 2 ||
		stop.Stop != fabricrunner.StopToolUse {
		t.Fatalf("usage/stop = %#v / %#v", usage, stop)
	}
}

func TestFinishReasonNormalization(t *testing.T) {
	t.Parallel()

	tests := map[string]fabricrunner.StopReason{
		"stop":           fabricrunner.StopEndTurn,
		"tool_calls":     fabricrunner.StopToolUse,
		"function_call":  fabricrunner.StopToolUse,
		"length":         fabricrunner.StopMaxTokens,
		"content_filter": fabricrunner.StopRefusal,
		"new_reason":     fabricrunner.StopUnknown,
	}
	for input, want := range tests {
		if got := normalizeFinishReason(input); got != want {
			t.Fatalf("finish reason %q = %q, want %q", input, got, want)
		}
	}
}

func TestStreamFailuresAndInStreamErrorRedaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		payload        string
		maxEvent       int
		wantErr        bool
		wantModelError bool
	}{
		{name: "missing done", payload: `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n", wantErr: true},
		{name: "malformed JSON", payload: "data: {\n\ndata: [DONE]\n\n", wantErr: true},
		{name: "multiple choices", payload: `data: {"choices":[{"index":0,"delta":{}},{"index":1,"delta":{}}]}` + "\n\ndata: [DONE]\n\n", wantErr: true},
		{name: "invalid call JSON", payload: `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call","type":"function","function":{"name":"tool","arguments":"{"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n", wantErr: true},
		{name: "negative usage", payload: `data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":-1,"completion_tokens":0}}` + "\n\ndata: [DONE]\n\n", wantErr: true},
		{name: "oversized", payload: "data: " + strings.Repeat("x", 100) + "\n\n", maxEvent: 20, wantErr: true},
		{name: "provider error", payload: `data: {"error":{"code":"secret-code","message":"super-secret"}}` + "\n\n", wantModelError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(response, test.payload)
				response.(http.Flusher).Flush()
			}))
			defer server.Close()
			maxBody := int64(0)
			if test.maxEvent > 0 {
				maxBody = 200
			}
			provider := mustProvider(t, Config{
				Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true,
				MaxEventBytes: test.maxEvent, MaxBodyBytes: maxBody,
			})
			stream, err := provider.Stream(context.Background(), baseRequest("test"))
			if err != nil {
				t.Fatal(err)
			}
			events, err := fabricrunner.DrainStream(context.Background(), stream)
			if test.wantErr && !errors.Is(err, ErrSSEProtocol) {
				t.Fatalf("error = %v, want ErrSSEProtocol", err)
			}
			if test.wantModelError {
				if err != nil || len(events) != 2 || events[1].Error == nil ||
					events[1].Error.Code != "provider_error" || strings.Contains(fmt.Sprint(events), "super-secret") {
					t.Fatalf("events = %#v, error = %v", events, err)
				}
			}
		})
	}
}

func TestStreamHTTPErrorDoesNotExposeBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(response, "super-secret response")
	}))
	defer server.Close()
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true,
		TokenSource: func(context.Context) (string, error) { return "super-secret", nil },
	})
	_, err := provider.Stream(context.Background(), baseRequest("test"))
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusTooManyRequests ||
		strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("HTTP error = %v", err)
	}
}

func TestCancellationAndIdempotentClose(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/event-stream")
		response.WriteHeader(http.StatusOK)
		response.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	transport := &closeCountingTransport{base: http.DefaultTransport}
	provider := mustProvider(t, Config{
		Name: "test", BaseURL: server.URL, AllowInsecureHTTP: true,
		HTTPClient: &http.Client{Transport: transport},
	})
	stream, err := provider.Stream(context.Background(), baseRequest("test"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if event, err := stream.Recv(context.Background()); err != nil || event.Type != fabricrunner.ModelEventStart {
		t.Fatalf("start event = %#v, %v", event, err)
	}
	result := make(chan error, 1)
	receiveCtx, cancel := context.WithCancel(context.Background())
	go func() {
		_, err := stream.Recv(receiveCtx)
		result <- err
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("receive error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock receive")
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if transport.closeCount.Load() != 1 {
		t.Fatalf("body close count = %d, want 1", transport.closeCount.Load())
	}
}

func TestProviderRunsInsideCanonicalToolLoop(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	var secondRequest map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			writeSSE(response, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"query\":\"go\"}"}}]},"finish_reason":"tool_calls"}]}`)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&secondRequest); err != nil {
			t.Errorf("decode second request: %v", err)
		}
		writeSSE(response, `{"choices":[{"index":0,"delta":{"content":"final"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	provider := mustProvider(t, Config{Name: "compatible", BaseURL: server.URL, AllowInsecureHTTP: true})
	selector := fixedSelector{provider: provider, model: fabricrunner.ModelRef{Provider: "compatible", Model: "model"}}
	tool := &adapterTool{}
	request := fabricrunner.LoopRequest{
		WorkloadID: newTestID(t), StepID: newTestID(t), AttemptID: newTestID(t),
		Initial: fabricrunner.ModelRequest{
			Model: selector.model,
			Messages: []fabricrunner.Message{{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{
				Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "question",
			}}}},
			ToolChoice: fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceAuto}, MaxOutputTokens: 100,
		},
		Tools: []fabricrunner.ToolBinding{{
			Definition: fabricrunner.ToolDefinition{
				Name: "lookup", InputSchema: json.RawMessage(`{"type":"object","required":["query"]}`),
				OutputSchema: json.RawMessage(`{"type":"object","required":["value"]}`),
			},
			Handler: tool,
		}},
		Selector: selector,
		Budget: fabricrunner.Budget{
			MaxInputTokens: 1000, MaxOutputTokens: 1000, MaxModelCalls: 3,
			MaxToolCalls: 2, MaxCost: 1000, MaxWallTime: time.Second,
		},
		ModelOutputClassification: fabricrunner.ClassInternal,
		ToolErrorClassification:   fabricrunner.ClassInternal,
	}
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stop != fabricrunner.StopEndTurn || result.ModelCalls != 2 || result.ToolCalls != 1 ||
		result.Messages[len(result.Messages)-1].Content[0].Text != "final" || tool.executeCount.Load() != 1 ||
		tool.closeCount.Load() != 1 {
		t.Fatalf("loop result = %#v, executes/closes = %d/%d", result, tool.executeCount.Load(), tool.closeCount.Load())
	}
	messages := secondRequest["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "tool" || last["tool_call_id"] != "call-1" ||
		last["content"] != `{"value":"result"}` {
		t.Fatalf("second-turn tool message = %#v", last)
	}
}

func baseRequest(provider string) fabricrunner.ModelRequest {
	return fabricrunner.ModelRequest{
		Model: fabricrunner.ModelRef{Provider: provider, Model: "model"},
		Messages: []fabricrunner.Message{{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{
			Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "hello",
		}}}},
		ToolChoice: fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceNone},
	}
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

func writeSSE(response http.ResponseWriter, chunks ...string) {
	response.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range chunks {
		_, _ = fmt.Fprintf(response, "data: %s\n\n", chunk)
	}
	_, _ = io.WriteString(response, "data: [DONE]\n\n")
	response.(http.Flusher).Flush()
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
