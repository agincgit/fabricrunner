// Package anthropic implements one-turn translation for the Anthropic Messages API.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/agincgit/fabricrunner"
)

const (
	defaultAPIVersion    = "2023-06-01"
	defaultMaxBodyBytes  = int64(4 << 20)
	defaultMaxEventBytes = 1 << 20
	defaultMaxPages      = 100
	defaultOutputTokens  = 4096
)

type AuthMode string

const (
	AuthBearer AuthMode = "bearer"
	AuthAPIKey AuthMode = "x-api-key"
)

type TokenSource func(context.Context) (string, error)

type Config struct {
	Name                string
	BaseURL             string
	AllowInsecureHTTP   bool
	AuthMode            AuthMode
	TokenSource         TokenSource
	APIVersion          string
	HTTPClient          *http.Client
	DefaultCapabilities fabricrunner.ModelCapabilities
	Labels              map[string]string
	DefaultOutputTokens int
	MaxBodyBytes        int64
	MaxEventBytes       int
	MaxModelPages       int
}

type Provider struct {
	name                string
	baseURL             url.URL
	authMode            AuthMode
	tokenSource         TokenSource
	apiVersion          string
	client              *http.Client
	defaultCapabilities fabricrunner.ModelCapabilities
	labels              map[string]string
	defaultOutputTokens int
	maxBodyBytes        int64
	maxEventBytes       int
	maxModelPages       int
}

type CredentialError struct{}

func (*CredentialError) Error() string { return "provider credential source failed" }

type HTTPError struct{ StatusCode int }

func (err *HTTPError) Error() string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("provider HTTP status %d", err.StatusCode)
}

var (
	ErrConfiguration = errors.New("invalid anthropic configuration")
	ErrTranslation   = errors.New("anthropic translation failed")
	ErrSSEProtocol   = errors.New("invalid anthropic event stream")
)

func New(config Config) (*Provider, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" || name != config.Name {
		return nil, fmt.Errorf("%w: provider name is empty or has surrounding whitespace", ErrConfiguration)
	}
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.BaseURL) != config.BaseURL {
		return nil, fmt.Errorf("%w: base URL is empty or has surrounding whitespace", ErrConfiguration)
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || !baseURL.IsAbs() || baseURL.Host == "" || baseURL.Opaque != "" ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || baseURL.RawPath != "" {
		return nil, fmt.Errorf("%w: base URL must be absolute without user information, query, or fragment", ErrConfiguration)
	}
	if baseURL.Scheme != "https" && (baseURL.Scheme != "http" || !config.AllowInsecureHTTP) {
		return nil, fmt.Errorf("%w: base URL must use HTTPS; plain HTTP requires explicit opt-in", ErrConfiguration)
	}
	baseURL.Path = strings.TrimSuffix(baseURL.Path, "/")
	authMode := config.AuthMode
	if authMode == "" {
		authMode = AuthBearer
	}
	if authMode != AuthBearer && authMode != AuthAPIKey {
		return nil, fmt.Errorf("%w: unknown authentication mode %q", ErrConfiguration, authMode)
	}
	apiVersion := config.APIVersion
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	if strings.TrimSpace(apiVersion) == "" || apiVersion != strings.TrimSpace(apiVersion) || strings.ContainsAny(apiVersion, "\r\n") {
		return nil, fmt.Errorf("%w: invalid API version", ErrConfiguration)
	}
	capabilities := cloneCapabilities(config.DefaultCapabilities)
	if capabilities.ToolUse == "" {
		capabilities.ToolUse = fabricrunner.ToolUseNone
	}
	descriptor := fabricrunner.ModelDescriptor{
		Ref: fabricrunner.ModelRef{Provider: name, Model: "configured"}, Capabilities: capabilities,
		Labels: cloneLabels(config.Labels),
	}
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("%w: defaults: %v", ErrConfiguration, err)
	}
	outputTokens := config.DefaultOutputTokens
	if outputTokens == 0 {
		outputTokens = defaultOutputTokens
	}
	if outputTokens < 1 {
		return nil, fmt.Errorf("%w: default output tokens must be positive", ErrConfiguration)
	}
	maxBody, maxEvent, maxPages := config.MaxBodyBytes, config.MaxEventBytes, config.MaxModelPages
	if maxBody == 0 {
		maxBody = defaultMaxBodyBytes
	}
	if maxEvent == 0 {
		maxEvent = defaultMaxEventBytes
	}
	if maxPages == 0 {
		maxPages = defaultMaxPages
	}
	if maxBody < 1 || maxEvent < 1 || maxPages < 1 {
		return nil, fmt.Errorf("%w: size and page limits must be positive", ErrConfiguration)
	}
	client := &http.Client{}
	if config.HTTPClient != nil {
		clone := *config.HTTPClient
		client = &clone
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Provider{name: name, baseURL: *baseURL, authMode: authMode, tokenSource: config.TokenSource,
		apiVersion: apiVersion, client: client, defaultCapabilities: descriptor.Capabilities,
		labels: descriptor.Labels, defaultOutputTokens: outputTokens, maxBodyBytes: maxBody,
		maxEventBytes: maxEvent, maxModelPages: maxPages}, nil
}

func (provider *Provider) Name() string { return provider.name }

func (provider *Provider) Models(ctx context.Context) ([]fabricrunner.ModelDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result []fabricrunner.ModelDescriptor
	seen, cursors := map[string]struct{}{}, map[string]struct{}{}
	after := ""
	for pageNumber := 0; pageNumber < provider.maxModelPages; pageNumber++ {
		values := url.Values{"limit": {"1000"}}
		if after != "" {
			values.Set("after_id", after)
		}
		request, err := provider.newRequest(ctx, http.MethodGet, "models", values, nil)
		if err != nil {
			return nil, err
		}
		response, err := provider.client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			failure := &HTTPError{StatusCode: response.StatusCode}
			_ = response.Body.Close()
			return nil, failure
		}
		var page struct {
			Data []struct {
				ID             string `json:"id"`
				MaxInputTokens *int   `json:"max_input_tokens"`
				MaxTokens      *int   `json:"max_tokens"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		err = decodeBoundedJSON(response.Body, provider.maxBodyBytes, &page)
		closeErr := response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("model discovery response: %w", err)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		for index, model := range page.Data {
			if _, exists := seen[model.ID]; exists {
				return nil, fmt.Errorf("model discovery response: duplicate model %q", model.ID)
			}
			seen[model.ID] = struct{}{}
			capabilities := cloneCapabilities(provider.defaultCapabilities)
			if model.MaxInputTokens != nil {
				capabilities.ContextTokens = *model.MaxInputTokens
			}
			if model.MaxTokens != nil {
				capabilities.MaxOutputTokens = *model.MaxTokens
			}
			descriptor := fabricrunner.ModelDescriptor{Ref: fabricrunner.ModelRef{Provider: provider.name, Model: model.ID}, Capabilities: capabilities, Labels: cloneLabels(provider.labels)}
			if err := descriptor.Validate(); err != nil {
				return nil, fmt.Errorf("model discovery response: model %d: %w", index, err)
			}
			result = append(result, descriptor)
		}
		if !page.HasMore {
			return result, nil
		}
		if page.LastID == "" {
			return nil, fmt.Errorf("model discovery response: paginated page has no last ID")
		}
		if _, exists := cursors[page.LastID]; exists {
			return nil, fmt.Errorf("model discovery response: pagination cycle")
		}
		cursors[page.LastID] = struct{}{}
		after = page.LastID
	}
	return nil, fmt.Errorf("model discovery response: pagination exceeds %d pages", provider.maxModelPages)
}

func (provider *Provider) Stream(ctx context.Context, request fabricrunner.ModelRequest) (fabricrunner.ModelStream, error) {
	payload, err := provider.translateRequest(request)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request", ErrTranslation)
	}
	streamCtx, cancel := context.WithCancel(ctx)
	httpRequest, err := provider.newRequest(streamCtx, http.MethodPost, "messages", nil, bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	response, err := provider.client.Do(httpRequest)
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		failure := &HTTPError{StatusCode: response.StatusCode}
		_ = response.Body.Close()
		cancel()
		return nil, failure
	}
	return newMessageStream(streamCtx, cancel, response.Body, provider.maxEventBytes), nil
}

func (provider *Provider) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	endpoint := provider.baseURL
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/" + path
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("anthropic-version", provider.apiVersion)
	if provider.tokenSource != nil {
		token, err := provider.tokenSource(ctx)
		if err != nil || strings.TrimSpace(token) == "" || token != strings.TrimSpace(token) || strings.ContainsAny(token, "\r\n") {
			return nil, &CredentialError{}
		}
		if provider.authMode == AuthAPIKey {
			request.Header.Set("x-api-key", token)
		} else {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return request, nil
}

func decodeBoundedJSON(reader io.Reader, limit int64, destination any) error {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("response exceeds configured size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("response contains trailing JSON")
	}
	return nil
}

func cloneLabels(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
func cloneCapabilities(input fabricrunner.ModelCapabilities) fabricrunner.ModelCapabilities {
	input.Modalities = append([]string(nil), input.Modalities...)
	return input
}

var _ fabricrunner.Provider = (*Provider)(nil)
