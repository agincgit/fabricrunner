package fabricrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrProviderCatalog     = errors.New("invalid provider catalog")
	ErrModelStreamProtocol = errors.New("invalid model stream protocol")
)

// DiscoverModels obtains and validates one provider's canonical model catalog.
func DiscoverModels(ctx context.Context, provider Provider) ([]ModelDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("%w: provider is nil", ErrProviderCatalog)
	}
	name := provider.Name()
	if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
		return nil, fmt.Errorf("%w: provider name is empty or has surrounding whitespace", ErrProviderCatalog)
	}
	descriptors, err := provider.Models(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]ModelDescriptor, len(descriptors))
	seen := make(map[string]struct{}, len(descriptors))
	for index, descriptor := range descriptors {
		if err := descriptor.Validate(); err != nil {
			return nil, fmt.Errorf("%w: model %d: %w", ErrProviderCatalog, index, err)
		}
		if descriptor.Ref.Provider != name {
			return nil, fmt.Errorf("%w: model %d belongs to provider %q, want %q",
				ErrProviderCatalog, index, descriptor.Ref.Provider, name)
		}
		if _, exists := seen[descriptor.Ref.Model]; exists {
			return nil, fmt.Errorf("%w: duplicate model %q", ErrProviderCatalog, descriptor.Ref.Model)
		}
		seen[descriptor.Ref.Model] = struct{}{}
		result[index] = descriptor.Clone()
	}
	return result, nil
}

// ModelStreamValidator validates the state of one canonical model stream.
// Its zero value is ready for use.
type ModelStreamValidator struct {
	lastSequence uint64
	started      bool
	terminal     bool
}

func (validator *ModelStreamValidator) Accept(event ModelEvent) error {
	if validator == nil {
		return fmt.Errorf("%w: validator is nil", ErrModelStreamProtocol)
	}
	if validator.terminal {
		return fmt.Errorf("%w: event followed terminal event", ErrModelStreamProtocol)
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrModelStreamProtocol, err)
	}
	if !validator.started {
		if event.Type != ModelEventStart {
			return fmt.Errorf("%w: first event is %q, want %q",
				ErrModelStreamProtocol, event.Type, ModelEventStart)
		}
		validator.started = true
	} else if event.Type == ModelEventStart {
		return fmt.Errorf("%w: repeated start event", ErrModelStreamProtocol)
	}
	if event.Sequence != validator.lastSequence+1 {
		return fmt.Errorf("%w: sequence %d followed %d",
			ErrModelStreamProtocol, event.Sequence, validator.lastSequence)
	}
	validator.lastSequence = event.Sequence
	validator.terminal = event.Type == ModelEventStop || event.Type == ModelEventError
	return nil
}

func (validator *ModelStreamValidator) Complete() error {
	if validator == nil {
		return fmt.Errorf("%w: validator is nil", ErrModelStreamProtocol)
	}
	if !validator.started {
		return fmt.Errorf("%w: stream ended before start", ErrModelStreamProtocol)
	}
	if !validator.terminal {
		return fmt.Errorf("%w: stream ended without a terminal event", ErrModelStreamProtocol)
	}
	return nil
}
