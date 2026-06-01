package vercel_test

import (
	"context"
	"fmt"

	"github.com/open-feature/go-sdk-contrib/providers/vercel"
	"github.com/open-feature/go-sdk/openfeature"
)

func Example() {
	provider, err := vercel.NewProvider(
		vercel.WithDatafile(vercel.Datafile{
			ProjectID:   "my-project",
			Environment: "production",
			Definitions: map[string]vercel.FlagDefinition{
				"new-checkout": {
					Environments: map[string]any{"production": 0},
					Variants:     []any{true, false},
				},
				"checkout-message": {
					Environments: map[string]any{
						"production": map[string]any{"fallthrough": 1},
					},
					Variants: []any{"Welcome!", "Checkout is live!"},
				},
			},
		}),
		vercel.WithPollingDisabled(),
	)
	if err != nil {
		panic(err)
	}

	err = openfeature.SetProviderAndWait(provider)
	if err != nil {
		panic(err)
	}
	defer openfeature.Shutdown()

	ctx := context.Background()
	client := openfeature.NewDefaultClient()

	evalCtx := openfeature.EvaluationContext{}
	enabled := client.Boolean(ctx, "new-checkout", false, evalCtx)
	message := client.String(ctx, "checkout-message", "default", evalCtx)

	fmt.Printf("new-checkout enabled: %v\n", enabled)
	fmt.Printf("checkout-message: %s\n", message)
	fmt.Printf("client state: %s\n", client.State())

	// Output:
	// new-checkout enabled: true
	// checkout-message: Checkout is live!
	// client state: READY
}
