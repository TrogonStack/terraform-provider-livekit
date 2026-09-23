# Schema Design

## Overview

Schemas define the shape of configuration, plan, and state data. Each attribute or block maps to a Go struct field via `tfsdk` tags.

```go
type agentSecretResourceModel struct {
    Id             types.String `tfsdk:"id"`
    AgentId        types.String `tfsdk:"agent_id"`
    Name           types.String `tfsdk:"name"`
    Kind           types.String `tfsdk:"kind"`
    ValueWo        types.String `tfsdk:"value_wo"`
    ValueWoVersion types.Int64  `tfsdk:"value_wo_version"`
    CreatedAt      types.String `tfsdk:"created_at"`
    UpdatedAt      types.String `tfsdk:"updated_at"`
}
```

## Attribute Types

### Primitives

| Schema Type               | Go Type         | Notes               |
| -------------------------- | ---------------- | --------------------- |
| `schema.StringAttribute`  | `types.String`  | UTF-8 string        |
| `schema.BoolAttribute`    | `types.Bool`    | true/false          |
| `schema.Int64Attribute`   | `types.Int64`   | 64-bit integer      |
| `schema.Int32Attribute`   | `types.Int32`   | 32-bit integer      |
| `schema.Float64Attribute` | `types.Float64` | 64-bit float        |
| `schema.Float32Attribute` | `types.Float32` | 32-bit float        |
| `schema.NumberAttribute`  | `types.Number`  | Arbitrary precision |

`livekit_agent_secret.value_wo_version` is a plain `schema.Int64Attribute{Required: true}`, backed by `types.Int64`.

### Collections

| Schema Type            | Go Type      | Requires      |
| ------------------------ | -------------- | --------------- |
| `schema.ListAttribute` | `types.List` | `ElementType` |
| `schema.MapAttribute`  | `types.Map`  | `ElementType` |
| `schema.SetAttribute`  | `types.Set`  | `ElementType` |

`livekit_agent.regions` is a `schema.SetAttribute` of strings, since region membership is unordered:

```go
"regions": schema.SetAttribute{
    Optional:    true,
    Computed:    true,
    ElementType: types.StringType,
    PlanModifiers: []planmodifier.Set{
        setplanmodifier.UseStateForUnknown(),
    },
}
```

### Nested Attributes (Protocol v6 only)

| Schema Type                    | Go Type                  | Use Case                |
| --------------------------------- | -------------------------- | -------------------------- |
| `schema.SingleNestedAttribute` | `*nestedModel`           | Single object           |
| `schema.ListNestedAttribute`   | `[]nestedModel`          | Ordered list of objects |
| `schema.MapNestedAttribute`    | `map[string]nestedModel` | Keyed objects           |
| `schema.SetNestedAttribute`    | `[]nestedModel`          | Unique set of objects   |

Neither resource in this provider uses nested attributes today; both schemas are flat. This table is here for when one is needed:

```go
schema.SingleNestedAttribute{
    Optional: true,
    Attributes: map[string]schema.Attribute{
        "key":   schema.StringAttribute{Required: true},
        "value": schema.StringAttribute{Required: true},
    },
}
```

## Blocks

Blocks are structural containers that appear as HCL blocks (with `{}` syntax). Use blocks for complex nested structures, especially when they can be optional or repeated.

| Schema Type                | Go Type                               | HCL Syntax                              |
| ----------------------------- | ---------------------------------------- | ------------------------------------------ |
| `schema.SingleNestedBlock` | `*nestedModel` (pointer for optional) | `block_name { ... }`                    |
| `schema.ListNestedBlock`   | `[]nestedModel`                       | `block_name { ... }` (repeated)         |
| `schema.SetNestedBlock`    | `[]nestedModel`                       | `block_name { ... }` (unique, repeated) |

This provider does not currently define any blocks; both `livekit_agent` and `livekit_agent_secret` are flat attribute bags. Reach for a block only if a future resource needs an optional, HCL-block-shaped nested structure.

### Blocks vs Nested Attributes

| Use Blocks When                                      | Use Nested Attributes When             |
| -------------------------------------------------------- | ------------------------------------------- |
| Optional complex object (pointer nil = not provided) | Always-present object structure        |
| Matching existing Terraform provider conventions     | New providers (preferred direction)    |
| HCL block syntax feels natural for the structure     | Programmatic, data-oriented structures |

## Attribute Behaviors

### Required, Optional, Computed

| Combination                                    | Meaning                                    |
| ------------------------------------------------- | ---------------------------------------------- |
| `Required: true`                               | User must provide; error if missing        |
| `Optional: true`                               | User may provide; null if omitted          |
| `Computed: true`                               | Provider sets the value; user cannot       |
| `Optional: true, Computed: true`               | User may provide OR provider fills         |
| `Optional: true, Computed: true, Default: ...` | User may provide; known default if omitted |

`livekit_agent.agent_id`, `agent_name`, `version`, and `deployed_at` are `Computed: true`. `regions` is `Optional: true, Computed: true` (no `Default`, since the provider does not know which region LiveKit Cloud will pick). `livekit_agent_secret.kind` is `Optional: true, Computed: true, Default: stringdefault.StaticString("environment")`.

### Sensitive

```go
schema.StringAttribute{
    Required:  true,
    Sensitive: true, // Value hidden in plan/state output
}
```

The provider's own `api_secret` attribute and `livekit_agent_secret.value_wo` are both `Sensitive: true`.

### Deprecation

```go
schema.StringAttribute{
    Optional:           true,
    DeprecationMessage: "Use 'new_field' instead.",
}
```

Not used anywhere in this provider yet.

## Defaults

Set a known value when the user does not provide one. Requires `Optional: true, Computed: true`.

```go
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"

schema.StringAttribute{
    Optional: true,
    Computed: true,
    Default:  stringdefault.StaticString(string(secretKindEnvironment)),
}
```

This is the actual `kind` attribute on `livekit_agent_secret` (`secretKindEnvironment` is defined in `secret_kind.go`).

## Plan Modifiers

Control how attribute values change during planning.

```go
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

schema.StringAttribute{
    Computed: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(), // ID: stable after creation
    },
}

schema.StringAttribute{
    Required: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(), // Immutable: forces recreation
    },
}
```

`livekit_agent_secret.agent_id`, `name`, and `kind` all use `stringplanmodifier.RequiresReplace()`, since changing any of them means a different secret, not an update to the current one.

## Validators

Constrain acceptable values at plan time.

```go
import "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
import "github.com/hashicorp/terraform-plugin-framework/schema/validator"

schema.StringAttribute{
    Optional: true,
    Computed: true,
    Default:  stringdefault.StaticString(string(secretKindEnvironment)),
    Validators: []validator.String{
        stringvalidator.OneOf(string(secretKindEnvironment), string(secretKindFile)),
    },
}
```

This is `livekit_agent_secret.kind`, restricted to `environment` or `file`.

## The `rsId()` Helper

This provider's standard ID attribute pattern, defined in `helpers.go`:

```go
func rsId() schema.StringAttribute {
    return schema.StringAttribute{
        Computed:            true,
        MarkdownDescription: "The unique ID of this resource.",
        PlanModifiers: []planmodifier.String{
            stringplanmodifier.UseStateForUnknown(),
        },
    }
}
```

Use `"id": rsId()` in every resource schema. `livekit_agent.id` is the agent ID; `livekit_agent_secret.id` is `<agent_id>/<name>`.

## Accessing Values from Models

```go
// Read plan/config into model
var plan agentResourceModel
resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

// Access primitive values
agentId := plan.Id.ValueString()

// Check null/unknown
if plan.Regions.IsNull() { /* user did not set */ }
if plan.Regions.IsUnknown() { /* will be known after apply */ }

// Access set elements
var regions []string
resp.Diagnostics.Append(plan.Regions.ElementsAs(ctx, &regions, false)...)

// Set values
plan.AgentId = types.StringValue("agent-123")
plan.Regions = types.SetValueMust(types.StringType, []attr.Value{})
```

## Related Framework References

| File                                                   | Contents                              |
| ---------------------------------------------------------- | ------------------------------------------ |
| `framework/handling-data/schemas.mdx`                  | Schema definition fundamentals        |
| `framework/handling-data/attributes/index.mdx`         | All attribute types overview          |
| `framework/handling-data/attributes/string.mdx`        | String attribute details              |
| `framework/handling-data/attributes/list-nested.mdx`   | List nested attribute                 |
| `framework/handling-data/attributes/single-nested.mdx` | Single nested attribute               |
| `framework/handling-data/blocks/index.mdx`             | Block types overview                  |
| `framework/handling-data/blocks/single-nested.mdx`     | SingleNestedBlock details             |
| `framework/handling-data/types/index.mdx`              | Go value types                        |
| `framework/handling-data/accessing-values.mdx`         | Reading values from state/plan/config |
| `framework/handling-data/writing-state.mdx`            | Writing values to state               |
| `framework/resources/default.mdx`                      | Default values                        |
