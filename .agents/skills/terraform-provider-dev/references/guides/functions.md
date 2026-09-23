# Provider-Defined Functions

## Overview

Provider-defined functions (Terraform 1.8+) let practitioners call provider logic directly in expressions. Unlike resources/data sources, functions are pure computations: no state, no side effects.

```hcl
# Usage in Terraform config:
output "parsed" {
  value = provider::googleworkspace::parse_email("user@example.com")
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

    "github.com/hashicorp/terraform-plugin-framework/function"
)

var _ function.Function = &parseEmailFunction{}

func newParseEmailFunction() function.Function {
    return &parseEmailFunction{}
}

type parseEmailFunction struct{}

func (f *parseEmailFunction) Metadata(_ context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
    resp.Name = "parse_email"
}

func (f *parseEmailFunction) Definition(_ context.Context, req function.DefinitionRequest, resp *function.DefinitionResponse) {
    resp.Definition = function.Definition{
        Summary:     "Parses an email address into local and domain parts",
        Description: "Given an email address, returns the local part (before @)",
        Parameters: []function.Parameter{
            function.StringParameter{
                Name:        "email",
                Description: "The email address to parse",
            },
        },
        Return: function.StringReturn{},
    }
}

func (f *parseEmailFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
    var email string
    resp.Error = function.ConcatFuncErrors(req.Arguments.Get(ctx, &email))
    if resp.Error != nil {
        return
    }

    // Parse logic
    parts := strings.SplitN(email, "@", 2)
    if len(parts) != 2 {
        resp.Error = function.NewFuncError("invalid email address: missing @")
        return
    }

    resp.Error = function.ConcatFuncErrors(resp.Result.Set(ctx, parts[0]))
}
```

### Register with Provider

Add to the provider's `Functions` method:

```go
var _ provider.ProviderWithFunctions = &googleworkspaceProvider{}

func (p *googleworkspaceProvider) Functions(_ context.Context) []func() function.Function {
    return []func() function.Function{
        newParseEmailFunction,
    }
}
```

## Parameter Types

| Parameter Type              | Go Argument Type              |
| --------------------------- | ----------------------------- |
| `function.StringParameter`  | `string`                      |
| `function.BoolParameter`    | `bool`                        |
| `function.Int64Parameter`   | `int64`                       |
| `function.Float64Parameter` | `float64`                     |
| `function.ListParameter`    | `[]T` or `types.List`         |
| `function.MapParameter`     | `map[string]T` or `types.Map` |
| `function.SetParameter`     | `[]T` or `types.Set`          |
| `function.ObjectParameter`  | struct or `types.Object`      |
| `function.DynamicParameter` | `types.Dynamic`               |

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
| ------------------------ | --------------- |
| `function.StringReturn`  | `string`        |
| `function.BoolReturn`    | `bool`          |
| `function.Int64Return`   | `int64`         |
| `function.Float64Return` | `float64`       |
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
func TestParseEmailFunction(t *testing.T) {
    f := &parseEmailFunction{}

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
  value = provider::googleworkspace::parse_email("user@example.com")
}
`,
            Check: resource.TestCheckOutput("test", "user"),
        },
    },
})
```

## Related Framework References

| File                                       | Contents               |
| ------------------------------------------ | ---------------------- |
| `framework/functions/index.mdx`            | Functions overview     |
| `framework/functions/concepts.mdx`         | Concepts and use cases |
| `framework/functions/implementation.mdx`   | Implementation details |
| `framework/functions/testing.mdx`          | Testing functions      |
| `framework/functions/errors.mdx`           | Error handling         |
| `framework/functions/parameters/index.mdx` | All parameter types    |
| `framework/functions/returns/index.mdx`    | All return types       |
