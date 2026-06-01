package vercel

import (
	"net/http"
	"testing"
	"time"
)

func TestWithHostTrimsTrailingSlash(t *testing.T) {
	o := defaultOptions()
	WithHost("https://example.com/")(&o)
	if o.host != "https://example.com" {
		t.Fatalf("expected https://example.com, got %q", o.host)
	}
}

func TestWithHostNoTrailingSlash(t *testing.T) {
	o := defaultOptions()
	WithHost("https://example.com")(&o)
	if o.host != "https://example.com" {
		t.Fatalf("expected https://example.com, got %q", o.host)
	}
}

func TestWithHostEmptyFallsBackToDefault(t *testing.T) {
	o := defaultOptions()
	WithHost("")(&o)
	if o.host != defaultHost {
		t.Fatalf("expected %q, got %q", defaultHost, o.host)
	}
}

func TestDefaultOptionsHostSet(t *testing.T) {
	o := defaultOptions()
	if o.host != defaultHost {
		t.Fatalf("expected %q, got %q", defaultHost, o.host)
	}
}

func TestWithSDKKey(t *testing.T) {
	o := defaultOptions()
	WithSDKKey("vf_server_key")(&o)
	if o.sdkKeyOrConnectionString != "vf_server_key" {
		t.Fatalf("expected vf_server_key, got %q", o.sdkKeyOrConnectionString)
	}
}

func TestWithConnectionStringDelegatesToSDKKey(t *testing.T) {
	o := defaultOptions()
	WithConnectionString("flags:key=vf_server_key")(&o)
	if o.sdkKeyOrConnectionString != "flags:key=vf_server_key" {
		t.Fatalf("expected connection string, got %q", o.sdkKeyOrConnectionString)
	}
}

func TestWithHTTPClient(t *testing.T) {
	o := defaultOptions()
	client := &http.Client{Timeout: time.Second}
	WithHTTPClient(client)(&o)
	if o.httpClient != client {
		t.Fatal("expected custom HTTP client")
	}
}

func TestWithHTTPClientNilIgnored(t *testing.T) {
	o := defaultOptions()
	original := o.httpClient
	WithHTTPClient(nil)(&o)
	if o.httpClient != original {
		t.Fatal("nil client should not replace existing client")
	}
}

func TestWithDatafile(t *testing.T) {
	o := defaultOptions()
	df := testDatafile()
	WithDatafile(df)(&o)
	if o.datafile == nil {
		t.Fatal("expected datafile to be set")
	}
	if o.datafile.Environment != "production" {
		t.Fatalf("expected production, got %q", o.datafile.Environment)
	}
}

func TestWithPollingIntervalEnablesPolling(t *testing.T) {
	o := defaultOptions()
	o.pollingEnabled = false
	WithPollingInterval(time.Minute)(&o)
	if !o.pollingEnabled {
		t.Fatal("expected polling enabled")
	}
	if o.pollingInterval != time.Minute {
		t.Fatalf("expected 1m, got %v", o.pollingInterval)
	}
}

func TestWithPollingDisabled(t *testing.T) {
	o := defaultOptions()
	WithPollingDisabled()(&o)
	if o.pollingEnabled {
		t.Fatal("expected polling disabled")
	}
}

func TestWithHostMultipleCalls(t *testing.T) {
	o := defaultOptions()
	WithHost("https://a.com")(&o)
	WithHost("https://b.com/")(&o)
	if o.host != "https://b.com" {
		t.Fatalf("expected https://b.com, got %q", o.host)
	}
}
