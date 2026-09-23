# Schema Design

## Overview

Schemas define the shape of configuration, plan, and state data. Each attribute or block maps to a Go struct field via `tfsdk` tags.

```go
type fooResourceModel struct {
    Id          types.String          `tfsdk:"id"`
    Name        types.String          `tfsdk:"name"`
    Enabled     types.Bool            `tfsdk:"enabled"`
    Tags        types.List            `tfsdk:"tags"`
    Settings    *settingsModel        `tfsdk:"settings"`
}

type settingsModel struct {
    MaxRetries types.Int64  `tfsdk:"max_retries"`
    Timeout    types.String `tfsdk:"timeout"`
}
```

## Attribute Types

### Primitives

| Schema Type               | Go Type         | Notes               |
| ------------------------- | --------------- | ------------------- |
| `schema.StringAttribute`  | `types.String`  | UTF-8 string        |
| `schema.BoolAttribute`    | `types.Bool`    | true/false          |
| `schema.Int64Attribute`   | `types.Int64`   | 64-bit integer      |
| `schema.Int32Attribute`   | `types.Int32`   | 32-bit integer      |
| `schema.Float64Attribute` | `types.Float64` | 64-bit float        |
| `schema.Float32Attribute` | `types.Float32` | 32-bit float        |
| `schema.NumberAttribute`  | `types.Number`  | Arbitrary precision |

### Collections

| Schema Type            | Go Type      | Requires      |
| ---------------------- | ------------ | ------------- |
| `schema.ListAttribute` | `types.List` | `ElementType` |
| `schema.MapAttribute`  | `types.Map`  | `ElementType` |
| `schema.SetAttribute`  | `types.Set`  | `ElementType` |

```go
schema.ListAttribute{
    Optional:    true,
    ElementType: types.StringType,
}
```

### Nested Attributes (Protocol v6 only)

| Schema Type                    | Go Type                  | Use Case                |
| ------------------------------ | ------------------------ | ----------------------- |
| `schema.SingleNestedAttribute` | `*nestedModel`           | Single object           |
| `schema.ListNestedAttribute`   | `[]nestedModel`          | Ordered list of objects |
| `schema.MapNestedAttribute`    | `map[string]nestedModel` | Keyed objects           |
| `schema.SetNestedAttribute`    | `[]nestedModel`          | Unique set of objects   |

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
| -------------------------- | ------------------------------------- | --------------------------------------- |
| `schema.SingleNestedBlock` | `*nestedModel` (pointer for optional) | `block_name { ... }`                    |
| `schema.ListNestedBlock`   | `[]nestedModel`                       | `block_name { ... }` (repeated)         |
| `schema.SetNestedBlock`    | `[]nestedModel`                       | `block_name { ... }` (unique, repeated) |

```go
resp.Schema = schema.Schema{
    Attributes: map[string]schema.Attribute{
        "id":   rsId(),
        "name": schema.StringAttribute{Required: true},
    },
    Blocks: map[string]schema.Block{
        "settings": schema.SingleNestedBlock{
            Attributes: map[string]schema.Attribute{
                "max_retries": schema.Int64Attribute{Optional: true},
                "timeout":     schema.StringAttribute{Optional: true},
            },
        },
    },
}
```

### Blocks vs Nested Attributes

| Use Blocks When                                      | Use Nested Attributes When             |
| ---------------------------------------------------- | -------------------------------------- |
| Optional complex object (pointer nil = not provided) | Always-present object structure        |
| Matching existing Terraform provider conventions     | New providers (preferred direction)    |
| HCL block syntax feels natural for the structure     | Programmatic, data-oriented structures |

In this provider, we use `schema.SingleNestedBlock` for optional nested objects (e.g., `restrictions` on drives).

## Attribute Behaviors

### Required, Optional, Computed

| Combination                                    | Meaning                                    |
| ---------------------------------------------- | ------------------------------------------ |
| `Required: true`                               | User must provide; error if missing        |
| `Optional: true`                               | User may provide; null if omitted          |
| `Computed: true`                               | Provider sets the value; user cannot       |
| `Optional: true, Computed: true`               | User may provide OR provider fills         |
| `Optional: true, Computed: true, Default: ...` | User may provide; known default if omitted |

### Sensitive

```go
schema.StringAttribute{
    Required:  true,
    Sensitive: true, // Value hidden in plan/state output
}
```

### Deprecation

```go
schema.StringAttribute{
    Optional:           true,
    DeprecationMessage: "Use 'new_field' instead.",
}
```

## Defaults

Set a known value when the user does not provide one. Requires `Optional: true, Computed: true`.

```go
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"

schema.BoolAttribute{
    Optional: true,
    Computed: true,
    Default:  booldefault.StaticBool(false),
}

schema.StringAttribute{
    Optional: true,
    Computed: true,
    Default:  stringdefault.StaticString("/"),
}

schema.Int64Attribute{
    Optional: true,
    Computed: true,
    Default:  int64default.StaticInt64(3),
}
```

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

## Validators

Constrain acceptable values at plan time.

```go
import "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
import "github.com/hashicorp/terraform-plugin-framework/schema/validator"

schema.StringAttribute{
    Required: true,
    Validators: []validator.String{
        stringvalidator.LengthBetween(1, 256),
        stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z]`), "must start with lowercase letter"),
    },
}
```

## The `rsId()` Helper

This provider's standard ID attribute pattern:

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

Use `"id": rsId()` in every resource schema.

## Accessing Values from Models

```go
// Read plan/config into model
var plan fooResourceModel
resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

// Access primitive values
name := plan.Name.ValueString()
enabled := plan.Enabled.ValueBool()
count := plan.Count.ValueInt64()

// Check null/unknown
if plan.Name.IsNull() { /* user did not set */ }
if plan.Name.IsUnknown() { /* will be known after apply */ }

// Access list elements
var tags []string
resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)

// Set values
plan.Id = types.StringValue("computed-id")
plan.Enabled = types.BoolValue(true)
plan.Tags = types.ListNull(types.StringType) // null list
```

## Related Framework References

| File                                                   | Contents                              |
| ------------------------------------------------------ | ------------------------------------- |
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
