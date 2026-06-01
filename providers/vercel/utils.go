package vercel

import (
	"encoding/json"

	"github.com/open-feature/go-sdk/openfeature"
)

func isNewerData(current, incoming *Datafile) bool {
	if current == nil {
		return true
	}
	if incoming == nil {
		return false
	}
	if current.Revision != 0 || incoming.Revision != 0 {
		return incoming.Revision > current.Revision
	}
	if current.Digest != "" && incoming.Digest != "" {
		return current.Digest != incoming.Digest
	}

	currentTime, currentOK := toFloat64(current.ConfigUpdatedAt)
	incomingTime, incomingOK := toFloat64(incoming.ConfigUpdatedAt)
	if currentOK && incomingOK {
		return incomingTime > currentTime
	}

	return true
}

func providerResolutionDetail(result evaluationResult) openfeature.ProviderResolutionDetail {
	if result.Reason == reasonError {
		return openfeature.ProviderResolutionDetail{
			Reason:          openfeature.ErrorReason,
			ResolutionError: resolutionError(result),
		}
	}

	return openfeature.ProviderResolutionDetail{
		Reason:  mapReason(result.Reason),
		Variant: result.Variant,
	}
}

func mapReason(reason vercelReason) openfeature.Reason {
	switch reason {
	case reasonPaused:
		return openfeature.StaticReason
	case reasonFallthrough:
		return openfeature.DefaultReason
	case reasonTargetMatch, reasonRuleMatch:
		return openfeature.TargetingMatchReason
	case reasonError:
		return openfeature.ErrorReason
	default:
		return openfeature.UnknownReason
	}
}

func resolutionError(result evaluationResult) openfeature.ResolutionError {
	switch result.ErrorCode {
	case "FLAG_NOT_FOUND":
		return openfeature.NewFlagNotFoundResolutionError(result.ErrorMessage)
	default:
		return openfeature.NewGeneralResolutionError(result.ErrorMessage)
	}
}

func typeMismatchDetail(message string) openfeature.ProviderResolutionDetail {
	return openfeature.ProviderResolutionDetail{
		Reason:          openfeature.ErrorReason,
		ResolutionError: openfeature.NewTypeMismatchResolutionError(message),
	}
}

func numericFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func numericInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), true
		}
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return i, true
		}
		if f, err := typed.Float64(); err == nil && f == float64(int64(f)) {
			return int64(f), true
		}
	}
	return 0, false
}
