# Provider-Defined Functions

## Overview

Provider-defined functions (Terraform 1.8+) let practitioners call provider logic directly in expressions. Unlike resources/data sources, functions are pure computations: no state, no side effects.

This provider defines no functions today. Everything below is illustrative, built around a plausible one: splitting a `livekit_agent_secret` compound ID (`<agent_id>/<name>`, the same format `parseAgentSecretImportID` in `helpers.go` already parses for import) into its two parts, so a practitioner could do this in an expression instead of only at import time:

```hcl
# Usage in Terraform config (illustrative, this function does not exist):
output "secret_agent_id" {
  value = provider::livekit::parse_agent_secret_id("agent-123/MY_SECRET").agent_id
}
```

## Interface

```go
type Function interface {
    Metadata(context.Context, MetadataRequest, *MetadataResponse)
    Definition(context.Context, DefinitionRequest, *DefinitionResponse)
    Run(context.Context, RunRequest, *RunResponse)
}
```

## Implementation

### Define the Function

```go
package provider

import (
    "context"
    "fmt"
    "strings"

    "github.com/hashicorp/terraform-plugin-framework/function"
    "github.com/hashicorp/terraform-plugin-framework/types"
)

var _ function.Function = &parseAgentSecretIdFunction{}

func newParseAgentSecretIdFunction() function.Function {
    return &parseAgentSecretIdFunction{}
}

type parseAgentSecretIdFunction struct{}

func (f *parseAgentSecretIdFunction) Metadata(_ context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
    resp.Name = "parse_agent_secret_id"
}

func (f *parseAgentSecretIdFunction) Definition(_ context.Context, req function.DefinitionRequest, resp *function.DefinitionResponse) {
    resp.Definition = function.Definition{
        Summary:     "Splits a livekit_agent_secret compound ID into its agent_id and name parts",
        Description: "Given an ID in the `<agent_id>/<name>` format, returns an object with agent_id and name attributes.",
        Parameters: []function.Parameter{
            function.StringParameter{
                Name:        "id",
                Description: "The compound ID, e.g. \"agent-123/MY_SECRET\"",
            },
        },
        Return: function.ObjectReturn{
            AttributeTypes: map[string]attr.Type{
                "agent_id": types.StringType,
                "name":     types.StringType,
            },
        },
    }
}

func (f *parseAgentSecretIdFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
    var id string
    resp.Error = function.ConcatFuncErrors(req.Arguments.Get(ctx, &id))
    if resp.Error != nil {
        return
    }

    parts := strings.SplitN(id, "/", 2)
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        resp.Error = function.NewArgumentFuncError(0, fmt.Sprintf("expected id in the format <agent_id>/<name>, got: %q", id))
        return
    }

    result, diags := types.ObjectValue(
        map[string]attr.Type{"agent_id": types.StringType, "name": types.StringType},
        map[string]attr.Value{"agent_id": types.StringValue(parts[0]), "name": types.StringValue(parts[1])},
    )
    resp.Error = function.ConcatFuncErrors(function.FuncErrorFromDiags(ctx, diags))
    if resp.Error != nil {
        return
    }
    resp.Error = function.ConcatFuncErrors(resp.Result.Set(ctx, result))
}
```

### Register with Provider

Add to the provider's `Functions` method:

```go
var _ provider.ProviderWithFunctions = &livekitProvider{}

func (p *livekitProvider) Functions(_ context.Context) []func() function.Function {
    return []func() function.Function{
        newParseAgentSecretIdFunction,
    }
}
```

`livekitProvider` does not implement `provider.ProviderWithFunctions` today; this would be a new addition to `provider.go`, alongside `Resources()` and `DataSources()`.

## Parameter Types

| Parameter Type              | Go Argument Type              |
| ------------------------------ | -------------------------------- |
| `function.StringParameter`  | `string`                      |
| `function.BoolParameter`    | `bool`                         |
| `function.Int64Parameter`   | `int64`                        |
| `function.Float64Parameter` | `float64`                       |
| `function.ListParameter`    | `[]T` or `types.List`         |
| `function.MapParameter`     | `map[string]T` or `types.Map` |
| `function.SetParameter`     | `[]T` or `types.Set`          |
| `function.ObjectParameter`  | struct or `types.Object`      |
| `function.DynamicParameter` | `types.Dynamic`                |

### Variadic Parameter

```go
resp.Definition = function.Definition{
    Parameters: []function.Parameter{
        function.StringParameter{Name: "separator"},
    },
    VariadicParameter: function.StringParameter{
        Name:        "values",
        Description: "Values to join",
    },
    Return: function.StringReturn{},
}

func (f *joinFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
    var separator string
    var values []string
    resp.Error = function.ConcatFuncErrors(req.Arguments.Get(ctx, &separator, &values))
    // ...
}
```

## Return Types

| Return Type              | Go Result Type  |
| --------------------------- | ------------------ |
| `function.StringReturn`  | `string`         |
| `function.BoolReturn`    | `bool`           |
| `function.Int64Return`   | `int64`          |
| `function.Float64Return` | `float64`        |
| `function.ListReturn`    | `types.List`    |
| `function.MapReturn`     | `types.Map`     |
| `function.SetReturn`     | `types.Set`     |
| `function.ObjectReturn`  | `types.Object`  |
| `function.DynamicReturn` | `types.Dynamic` |

## Error Handling

Functions use `function.FuncError` instead of diagnostics:

```go
// Single error
resp.Error = function.NewFuncError("something went wrong")

// Error with argument position
resp.Error = function.NewArgumentFuncError(0, "first argument is invalid")

// Combine errors
resp.Error = function.ConcatFuncErrors(
    req.Arguments.Get(ctx, &arg1, &arg2),
)
```

## Testing Functions

### Unit Tests

```go
func TestParseAgentSecretIdFunction(t *testing.T) {
    f := &parseAgentSecretIdFunction{}

    // Test definition
    defResp := function.DefinitionResponse{}
    f.Definition(context.Background(), function.DefinitionRequest{}, &defResp)
    if defResp.Definition.Summary == "" {
        t.Error("expected non-empty summary")
    }
}
```

### Acceptance Tests

```go
resource.Test(t, resource.TestCase{
    ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
    Steps: []resource.TestStep{
        {
            Config: testProviderConfig + `
output "test" {
  value = provider::livekit::parse_agent_secret_id("agent-123/MY_SECRET").agent_id
}
`,
            Check: resource.TestCheckOutput("test", "agent-123"),
        },
    },
})
```

## Related Framework References

| File                                       | Contents               |
| ----------------------------------------------- | ------------------------- |
| `framework/functions/index.mdx`            | Functions overview     |
| `framework/functions/concepts.mdx`         | Concepts and use cases |
| `framework/functions/implementation.mdx`   | Implementation details |
| `framework/functions/testing.mdx`          | Testing functions      |
| `framework/functions/errors.mdx`           | Error handling         |
| `framework/functions/parameters/index.mdx` | All parameter types    |
| `framework/functions/returns/index.mdx`    | All return types       |
