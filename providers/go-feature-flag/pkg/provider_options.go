package gofeatureflag

import ffclient "github.com/thomaspoignant/go-feature-flag"

// ProviderOptions is the struct containing the provider options you can
// use while initializing GO Feature Flag.
// To have a valid configuration you need to have at Endpoint or GOFeatureFlagConfig to be set.
type ProviderOptions struct {
	// Endpoint contains the DNS of your GO Feature Flag relay proxy (ex: http://localhost:1031)
	Endpoint string

	// HTTPClient (optional) is the HTTP Client we will use to contact GO Feature Flag.
	// By default, we are using a custom HTTPClient with a timeout configure to 10000 milliseconds.
	HTTPClient HTTPClient

	// GOFeatureFlagConfig is the configuration struct for the GO Feature Flag module.
	// If not nil we will launch the provider using the GO Feature Flag module.
	GOFeatureFlagConfig *ffclient.Config
}
