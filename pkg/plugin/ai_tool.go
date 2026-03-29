package plugin

import (
	"context"

	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/model"
)

// AIToolDefinition describes a tool that a plugin exposes to the Kite AI agent.
// When a user asks the AI a question, the agent can invoke these tools to
// retrieve data or perform actions on behalf of the user.
//
// The schema follows the same JSON-Schema conventions used by Kite's built-in
// tools (see pkg/ai/tools.go agentToolDefinition).
type AIToolDefinition struct {
	// Name is the tool identifier. It will be prefixed with "plugin_<pluginName>_"
	// when registered in the AI agent to avoid collisions.
	Name string `json:"name" yaml:"name"`

	// Description explains what the tool does. The LLM reads this to decide
	// when to invoke the tool, so make it clear and specific.
	Description string `json:"description" yaml:"description"`

	// Properties is a JSON Schema object describing the tool's parameters.
	// Each key is a parameter name, value is {"type": "string", "description": "..."}.
	Properties map[string]any `json:"properties" yaml:"properties"`

	// Required lists parameter names that must be provided by the AI.
	Required []string `json:"required" yaml:"required"`
}

// AIToolExecutor is the function signature for executing an AI tool.
// The plugin receives the Kubernetes ClientSet for the current cluster
// and the parsed arguments from the AI agent.
//
// It returns the result as a string (rendered to the AI) and an error.
// If the error is non-nil, the AI treats the result as an error message.
type AIToolExecutor func(ctx context.Context, cs *cluster.ClientSet, args map[string]any) (string, error)

// AIToolAuthorizer is the function signature for checking whether a user
// has permission to invoke a specific AI tool with the given arguments.
//
// Return nil to allow, or an error to deny with a reason.
type AIToolAuthorizer func(user model.User, cs *cluster.ClientSet, args map[string]any) error

// AITool combines a definition with its runtime executor and authorizer.
// Plugins return a slice of these from RegisterAITools().
type AITool struct {
	Definition AIToolDefinition
	Execute    AIToolExecutor
	Authorize  AIToolAuthorizer
}
