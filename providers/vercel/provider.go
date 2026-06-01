package vercel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	of "github.com/open-feature/go-sdk/openfeature"
	"golang.org/x/sync/errgroup"
)

var (
	_ of.FeatureProvider          = (*Provider)(nil)
	_ of.ContextAwareStateHandler = (*Provider)(nil)
	_ of.EventHandler             = (*Provider)(nil)

	errMissingSDKKey      = errors.New("@vercel/flags-core: Missing sdkKey")
	errPollingInterval    = errors.New("@vercel/flags-core: Polling interval must be greater than 0")
	errMissingEnvVarFlags = errors.New("@vercel/flags: Missing environment variable FLAGS")
	errMissingDefinitions = errors.New("@vercel/flags-core: Invalid datafile: missing definitions")
	errMissingEnvironment = errors.New("@vercel/flags-core: Invalid datafile: missing environment")
)

type flagTypes interface {
	int64 | float64 | string | bool | any
}

// Provider is an OpenFeature provider backed by Vercel Flags.
type Provider struct {
	options providerOptions
	sdkKey  string

	initMu sync.Mutex
	mu     sync.RWMutex

	data        atomic.Pointer[Datafile]
	initialized bool
	status      of.State

	events     chan of.Event
	pollCancel context.CancelFunc
	pollWG     *errgroup.Group
}

// NewProvider creates a Vercel Flags OpenFeature provider.
//
// If WithSDKKey or WithConnectionString is not provided, NewProvider reads the
// FLAGS environment variable, matching the TypeScript VercelProvider default.
func NewProvider(opts ...Option) (*Provider, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	if options.pollingEnabled && options.pollingInterval <= 0 {
		return nil, errPollingInterval
	}

	var sdkKey string
	if options.sdkKeyOrConnectionString != "" {
		var ok bool
		sdkKey, ok = ParseSDKKey(options.sdkKeyOrConnectionString)
		if !ok {
			return nil, errMissingSDKKey
		}
	}

	if sdkKey == "" && options.datafile == nil {
		return nil, errMissingEnvVarFlags
	}
	if sdkKey == "" {
		options.pollingEnabled = false
	}

	provider := &Provider{
		options: options,
		sdkKey:  sdkKey,
		status:  of.NotReadyState,
		events:  make(chan of.Event, 5),
	}

	if options.datafile != nil {
		datafile := *options.datafile
		provider.data.Store(&datafile)
	}

	return provider, nil
}

// Metadata returns provider metadata.
func (p *Provider) Metadata() of.Metadata {
	return of.Metadata{Name: providerName}
}

// Hooks returns provider hooks. The Vercel provider does not install hooks.
func (p *Provider) Hooks() []of.Hook {
	return nil
}

// Init implements openfeature.StateHandler.
func (p *Provider) Init(evaluationContext of.EvaluationContext) error {
	return p.InitWithContext(context.Background(), evaluationContext)
}

// InitWithContext implements openfeature.ContextAwareStateHandler.
//
// ProviderReady is not emitted through the provider's own event channel
// here; the SDK emits it via its initialization flow. Direct consumers
// of EventChannel() will not see a ProviderReady event from this path.
func (p *Provider) InitWithContext(ctx context.Context, evaluationContext of.EvaluationContext) error {
	p.mu.RLock()
	if p.initialized {
		p.mu.RUnlock()
		return nil
	}
	p.mu.RUnlock()

	p.initMu.Lock()
	defer p.initMu.Unlock()

	p.mu.RLock()
	if p.initialized {
		p.mu.RUnlock()
		return nil
	}
	hasData := p.data.Load() != nil
	p.mu.RUnlock()

	var (
		data *Datafile
		err  error
	)
	if !hasData {
		data, err = p.fetchDatafile(ctx)
	}

	p.mu.Lock()
	if data != nil {
		p.data.Store(data)
		p.status = of.ReadyState
	}
	if err != nil {
		p.status = of.ErrorState
		err = &of.ProviderInitError{ErrorCode: of.GeneralCode, Message: err.Error()}
	}
	p.initialized = true
	p.startPollingLocked()
	p.mu.Unlock()

	return err
}

// Shutdown implements openfeature.StateHandler.
func (p *Provider) Shutdown() {
	_ = p.ShutdownWithContext(context.Background())
}

// ShutdownWithContext implements openfeature.ContextAwareStateHandler.
func (p *Provider) ShutdownWithContext(ctx context.Context) error {
	p.mu.Lock()
	cancel := p.pollCancel
	pollWG := p.pollWG
	p.pollCancel = nil
	p.pollWG = nil
	p.initialized = false
	p.status = of.NotReadyState
	p.mu.Unlock()

	if cancel == nil {
		// already shut down or never initialized
		return nil
	}
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- pollWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns the provider state.
func (p *Provider) Status() of.State {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// EventChannel implements openfeature.EventHandler.
func (p *Provider) EventChannel() <-chan of.Event {
	return p.events
}

func (p *Provider) BooleanEvaluation(ctx context.Context, flag string, defaultValue bool, flatCtx of.FlattenedContext) of.BoolResolutionDetail {
	return resolve(p.data.Load(), flag, defaultValue, flatCtx, func(a any) (bool, bool) {
		value, ok := a.(bool)
		return value, ok
	})
}

func (p *Provider) StringEvaluation(ctx context.Context, flag string, defaultValue string, flatCtx of.FlattenedContext) of.StringResolutionDetail {
	return resolve(p.data.Load(), flag, defaultValue, flatCtx, func(a any) (string, bool) {
		value, ok := a.(string)
		return value, ok
	})
}

func (p *Provider) FloatEvaluation(ctx context.Context, flag string, defaultValue float64, flatCtx of.FlattenedContext) of.FloatResolutionDetail {
	return resolve(p.data.Load(), flag, defaultValue, flatCtx, numericFloat64)
}

func (p *Provider) IntEvaluation(ctx context.Context, flag string, defaultValue int64, flatCtx of.FlattenedContext) of.IntResolutionDetail {
	return resolve(p.data.Load(), flag, defaultValue, flatCtx, numericInt64)
}

func (p *Provider) ObjectEvaluation(ctx context.Context, flag string, defaultValue any, flatCtx of.FlattenedContext) of.InterfaceResolutionDetail {
	return resolve(p.data.Load(), flag, defaultValue, flatCtx, func(a any) (any, bool) { return a, true })
}

// resolve evaluates a flag against the datafile and converts the result to the target type.
func resolve[T flagTypes](data *Datafile, flag string, defaultValue T, flatCtx of.FlattenedContext, convert func(any) (T, bool)) of.GenericResolutionDetail[T] {
	result := evaluateDatafile(data, flag, defaultValue, flatCtx)
	detail := providerResolutionDetail(result)

	if detail.Reason == of.ErrorReason {
		return of.GenericResolutionDetail[T]{Value: defaultValue, ProviderResolutionDetail: detail}
	}

	value, ok := convert(result.Value)
	if !ok {
		return of.GenericResolutionDetail[T]{
			Value: defaultValue,
			ProviderResolutionDetail: typeMismatchDetail(
				fmt.Sprintf(`Expected %T value for flag "%s"`, defaultValue, flag),
			),
		}
	}
	return of.GenericResolutionDetail[T]{
		Value:                    value,
		ProviderResolutionDetail: detail,
	}
}

func (p *Provider) fetchDatafile(ctx context.Context) (*Datafile, error) {
	if p.sdkKey == "" {
		return nil, errMissingSDKKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.options.host+"/v1/datafile", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.sdkKey)
	req.Header.Set("User-Agent", "VercelFlagsGo/0.1")
	if vercelEnv := os.Getenv("VERCEL_ENV"); vercelEnv != "" {
		req.Header.Set("X-Vercel-Env", vercelEnv)
	}

	res, err := p.options.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("failed to fetch data: %s", res.Status)
	}

	decoder := json.NewDecoder(res.Body)
	decoder.UseNumber()

	var datafile Datafile
	if err := decoder.Decode(&datafile); err != nil {
		return nil, err
	}
	if datafile.Definitions == nil {
		return nil, errMissingDefinitions
	}
	if datafile.Environment == "" {
		return nil, errMissingEnvironment
	}
	return &datafile, nil
}

func (p *Provider) startPollingLocked() {
	if !p.options.pollingEnabled || p.sdkKey == "" || p.pollCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	eg, egCtx := errgroup.WithContext(ctx)
	p.pollCancel = cancel
	p.pollWG = eg
	eg.Go(func() error {
		p.poll(egCtx, p.events)
		return nil
	})
}

func (p *Provider) poll(ctx context.Context, events chan of.Event) {
	ticker := time.NewTicker(p.options.pollingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			datafile, err := p.fetchDatafile(ctx)
			if err != nil {
				p.setStatus(of.StaleState)
				emit(events, of.ProviderStale, err.Error())
				continue
			}

			if p.replaceDataIfNewer(datafile) {
				emit(events, of.ProviderConfigChange, "")
			}
			if p.Status() != of.ReadyState {
				p.setStatus(of.ReadyState)
				emit(events, of.ProviderReady, "")
			}
		}
	}
}

func (p *Provider) replaceDataIfNewer(data *Datafile) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !isNewerData(p.data.Load(), data) {
		return false
	}
	p.data.Store(data)
	return true
}

func (p *Provider) setStatus(status of.State) {
	p.mu.Lock()
	p.status = status
	p.mu.Unlock()
}

// emit sends a provider event to the channel without blocking.
func emit(events chan<- of.Event, eventType of.EventType, message string) {
	event := of.Event{
		ProviderName: providerName,
		EventType:    eventType,
		ProviderEventDetails: of.ProviderEventDetails{
			Message: message,
		},
	}

	select {
	case events <- event:
	default:
	}
}
