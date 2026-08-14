package orchestration

import (
	"context"
	"fmt"
)

type Driver interface {
	Name() string
	Preflight(ctx context.Context) (Capabilities, error)
	Dispatch(ctx context.Context, request DispatchRequest, opts DispatchOptions) (ProviderHandle, error)
	Status(ctx context.Context, handle ProviderHandle) (ProviderExecutionStatus, error)
	Wait(ctx context.Context, handle ProviderHandle, timeoutSeconds int) error
	Finalize(ctx context.Context, handle ProviderHandle, request DispatchRequest, stateDir string) (ProviderResult, error)
}

type DriverFactory func(options map[string]any) (Driver, error)

type Registry struct {
	factories map[string]DriverFactory
}

func NewRegistry() *Registry {
	return &Registry{factories: map[string]DriverFactory{}}
}

func (r *Registry) Register(name string, factory DriverFactory) {
	r.factories[name] = factory
}

func (r *Registry) Driver(name string, options map[string]any) (Driver, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("unsupported orchestration provider %q", name)
	}
	return factory(options)
}
