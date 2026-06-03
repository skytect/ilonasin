package provider

type RoutePolicy struct {
	Responses ResponsesRoutePolicy
	Anthropic AnthropicRoutePolicy
	Stream    StreamRoutePolicy
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
		}
	case "openrouter":
		return RoutePolicy{
			Responses: ResponsesRoutePolicy{
				AllowParallelTools: true,
			},
			Anthropic: AnthropicRoutePolicy{
				IncludeGenerationOptions: true,
			},
		}
	default:
		return RoutePolicy{
			Anthropic: AnthropicRoutePolicy{
				IncludeGenerationOptions: true,
			},
		}
	}
}
