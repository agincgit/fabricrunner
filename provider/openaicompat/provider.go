// Package openaicompat implements one-turn OpenAI-compatible Chat Completions
// translation for hosted and personal-network endpoints.
package openaicompat

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
	defaultMaxBodyBytes  int64 = 4 << 20
	defaultMaxEventBytes       = 1 << 20
)

type TokenLimitField string

const (
	TokenLimitMaxTokens           TokenLimitField = "max_tokens"
	TokenLimitMaxCompletionTokens TokenLimitField = "max_completion_tokens"
)

type TokenSource func(context.Context) (string, error)

type Config struct {
	Name                string
	BaseURL             string
	AllowInsecureHTTP   bool
	TokenSource         TokenSource
	HTTPClient          *http.Client
	DefaultCapabilities fabricrunner.ModelCapabilities
	Labels              map[string]string
	TokenLimitField     TokenLimitField
	MaxBodyBytes        int64
	MaxEventBytes       int
}

type Provider struct {
	name                string
	baseURL             url.URL
	tokenSource         TokenSource
	client              *http.Client
	defaultCapabilities fabricrunner.ModelCapabilities
	labels              map[string]string
	tokenLimitField     TokenLimitField
	maxBodyBytes        int64
	maxEventBytes       int
}

type CredentialError struct{}

func (*CredentialError) Error() string { return "provider credential source failed" }

type HTTPError struct {
	StatusCode int
}

func (err *HTTPError) Error() string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("provider HTTP status %d", err.StatusCode)
}

var (
	ErrConfiguration = errors.New("invalid OpenAI-compatible configuration")
	ErrTranslation   = errors.New("OpenAI-compatible translation failed")
	ErrSSEProtocol   = errors.New("invalid OpenAI-compatible event stream")
)

func New(config Config) (*Provider, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" || name != config.Name {
		return nil, fmt.Errorf("%w: provider name is empty or has surrounding whitespace", ErrConfiguration)
	}
	if strings.TrimSpace(config.BaseURL) == "" || config.BaseURL != strings.TrimSpace(config.BaseURL) {
		return nil, fmt.Errorf("%w: base URL is empty or has surrounding whitespace", ErrConfiguration)
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: base URL: %v", ErrConfiguration, err)
	}
	if !baseURL.IsAbs() || baseURL.Host == "" || baseURL.Opaque != "" ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || baseURL.RawPath != "" {
		return nil, fmt.Errorf("%w: base URL must be absolute without user information, query, or fragment", ErrConfiguration)
	}
	switch baseURL.Scheme {
	case "https":
	case "http":
		if !config.AllowInsecureHTTP {
			return nil, fmt.Errorf("%w: plain HTTP requires explicit opt-in", ErrConfiguration)
		}
	default:
		return nil, fmt.Errorf("%w: base URL scheme must be HTTPS or HTTP", ErrConfiguration)
	}
	baseURL.Path = strings.TrimSuffix(baseURL.Path, "/")

	capabilities := config.DefaultCapabilities
	capabilities.Modalities = append([]string(nil), capabilities.Modalities...)
	if capabilities.ToolUse == "" {
		capabilities.ToolUse = fabricrunner.ToolUseNone
	}
	configuredDescriptor := fabricrunner.ModelDescriptor{
		Ref:          fabricrunner.ModelRef{Provider: name, Model: "configured"},
		Capabilities: capabilities,
		Labels:       cloneLabels(config.Labels),
	}
	if err := configuredDescriptor.Validate(); err != nil {
		return nil, fmt.Errorf("%w: defaults: %w", ErrConfiguration, err)
	}
	field := config.TokenLimitField
	if field == "" {
		field = TokenLimitMaxTokens
	}
	if field != TokenLimitMaxTokens && field != TokenLimitMaxCompletionTokens {
		return nil, fmt.Errorf("%w: unknown token-limit field %q", ErrConfiguration, field)
	}
	maxBodyBytes := config.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	if maxBodyBytes < 1 {
		return nil, fmt.Errorf("%w: maximum body bytes must be positive", ErrConfiguration)
	}
	maxEventBytes := config.MaxEventBytes
	if maxEventBytes == 0 {
		maxEventBytes = defaultMaxEventBytes
	}
	if maxEventBytes < 1 {
		return nil, fmt.Errorf("%w: maximum event bytes must be positive", ErrConfiguration)
	}

	client := &http.Client{}
	if config.HTTPClient != nil {
		clone := *config.HTTPClient
		client = &clone
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Provider{
		name: name, baseURL: *baseURL, tokenSource: config.TokenSource, client: client,
		defaultCapabilities: configuredDescriptor.Capabilities,
		labels:              configuredDescriptor.Labels,
		tokenLimitField:     field, maxBodyBytes: maxBodyBytes, maxEventBytes: maxEventBytes,
	}, nil
}

func (provider *Provider) Name() string { return provider.name }

func (provider *Provider) Models(ctx context.Context) ([]fabricrunner.ModelDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := provider.newRequest(ctx, http.MethodGet, "models", nil)
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
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, responseError(response)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := decodeBoundedJSON(response.Body, provider.maxBodyBytes, &payload); err != nil {
		return nil, fmt.Errorf("model discovery response: %w", err)
	}
	descriptors := make([]fabricrunner.ModelDescriptor, len(payload.Data))
	seen := make(map[string]struct{}, len(payload.Data))
	for index, model := range payload.Data {
		if _, exists := seen[model.ID]; exists {
			return nil, fmt.Errorf("model discovery response: duplicate model %q", model.ID)
		}
		seen[model.ID] = struct{}{}
		descriptors[index] = fabricrunner.ModelDescriptor{
			Ref:          fabricrunner.ModelRef{Provider: provider.name, Model: model.ID},
			Capabilities: cloneCapabilities(provider.defaultCapabilities),
			Labels:       cloneLabels(provider.labels),
		}
		if err := descriptors[index].Validate(); err != nil {
			return nil, fmt.Errorf("model discovery response: model %d: %w", index, err)
		}
	}
	return descriptors, nil
}

func (provider *Provider) Stream(
	ctx context.Context,
	request fabricrunner.ModelRequest,
) (fabricrunner.ModelStream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, err := provider.translateRequest(request)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request: %v", ErrTranslation, err)
	}
	streamCtx, cancel := context.WithCancel(ctx)
	httpRequest, err := provider.newRequest(streamCtx, http.MethodPost, "chat/completions", bytes.NewReader(body))
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
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		failure := responseError(response)
		response.Body.Close()
		cancel()
		return nil, failure
	}
	return newChatStream(streamCtx, cancel, response.Body, provider.maxEventBytes), nil
}

func (provider *Provider) newRequest(
	ctx context.Context,
	method, path string,
	body io.Reader,
) (*http.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	endpoint := provider.baseURL
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/" + path
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	if provider.tokenSource != nil {
		token, err := provider.tokenSource(ctx)
		if err != nil {
			return nil, &CredentialError{}
		}
		if strings.TrimSpace(token) == "" || token != strings.TrimSpace(token) || strings.ContainsAny(token, "\r\n") {
			return nil, &CredentialError{}
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request, nil
}

func responseError(response *http.Response) error {
	return &HTTPError{StatusCode: response.StatusCode}
}

func decodeBoundedJSON(reader io.Reader, limit int64, destination any) error {
	limited := io.LimitReader(reader, limit+1)
	data, err := io.ReadAll(limited)
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

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	return result
}

func cloneCapabilities(capabilities fabricrunner.ModelCapabilities) fabricrunner.ModelCapabilities {
	capabilities.Modalities = append([]string(nil), capabilities.Modalities...)
	return capabilities
}

var _ fabricrunner.Provider = (*Provider)(nil)
