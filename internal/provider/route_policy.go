package provider

type RoutePolicy struct {
	Responses    ResponsesRoutePolicy
	Anthropic    AnthropicRoutePolicy
	Stream       StreamRoutePolicy
	ChatMetadata ChatOptionMetadataPolicy
}

type ResponsesRoutePolicy struct {
	PreserveResponsesInput bool
	PreserveResponsesTools bool
	AllowProviderOptions   bool
	AllowParallelTools     bool
}

type AnthropicRoutePolicy struct {
	IncludeGenerationOptions bool
}

type StreamRoutePolicy struct {
	ExposeProviderErrorClasses bool
}

func RoutePolicyForInstance(instance Instance) RoutePolicy {
	switch instance.Type {
	case "codex":
		return RoutePolicy{
			Responses: ResponsesRoutePolicy{
				PreserveResponsesInput: true,
				PreserveResponsesTools: true,
				AllowProviderOptions:   true,
			},
			Anthropic: AnthropicRoutePolicy{
				IncludeGenerationOptions: false,
			},
			Stream: StreamRoutePolicy{
				ExposeProviderErrorClasses: true,
			},
			ChatMetadata: ChatOptionMetadataPolicy{codex: true, suppressCodexDefaultTier: true},
		}
	case "openrouter":
		return RoutePolicy{
			Responses: ResponsesRoutePolicy{
				AllowParallelTools: true,
			},
			Anthropic: AnthropicRoutePolicy{
				IncludeGenerationOptions: true,
			},
			ChatMetadata: ChatOptionMetadataPolicy{openrouter: true},
		}
	case "deepseek":
		return RoutePolicy{
			Anthropic: AnthropicRoutePolicy{
				IncludeGenerationOptions: true,
			},
			ChatMetadata: ChatOptionMetadataPolicy{deepseek: true},
		}
	default:
		return RoutePolicy{
			Anthropic: AnthropicRoutePolicy{
				IncludeGenerationOptions: true,
			},
		}
	}
}
