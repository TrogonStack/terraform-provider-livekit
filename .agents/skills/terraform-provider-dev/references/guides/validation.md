# Validation

## Overview

Validation runs during `terraform validate`, `terraform plan`, and `terraform apply`. It returns diagnostics (warnings/errors) before any API calls happen. Validation occurs at two levels:

1. **Attribute-level validators** — validate individual attribute values
2. **Resource/data-source-level validation** — cross-attribute validation logic

Important: configuration values may be unknown during validation (references to other resources). Validators must handle this by returning early without diagnostics.

## Attribute Validators

Add validators to any attribute's `Validators` field. All validators in the slice always run (no short-circuit).

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/schema/validator"
    "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
    "github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
    "github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
)

schema.StringAttribute{
    Required: true,
    Validators: []validator.String{
        stringvalidator.LengthBetween(1, 256),
        stringvalidator.RegexMatches(
            regexp.MustCompile(`^[a-z][a-z0-9-]*$`),
            "must start with lowercase letter, contain only lowercase alphanumeric and hyphens",
        ),
    },
}

schema.Int64Attribute{
    Optional: true,
    Validators: []validator.Int64{
        int64validator.Between(1, 100),
    },
}

schema.ListAttribute{
    Optional:    true,
    ElementType: types.StringType,
    Validators: []validator.List{
        listvalidator.SizeAtMost(10),
    },
}
```

## Common Validators (terraform-plugin-framework-validators)

### String

| Validator                                     | Description           |
| --------------------------------------------- | --------------------- |
| `stringvalidator.LengthBetween(min, max)`     | String length range   |
| `stringvalidator.LengthAtLeast(min)`          | Minimum length        |
| `stringvalidator.LengthAtMost(max)`           | Maximum length        |
| `stringvalidator.RegexMatches(re, msg)`       | Regex pattern match   |
| `stringvalidator.OneOf("a", "b", "c")`        | Enum values           |
| `stringvalidator.NoneOf("x", "y")`            | Excluded values       |
| `stringvalidator.UTF8LengthBetween(min, max)` | UTF-8 character count |
| `stringvalidator.IsURLWithHTTPS()`            | Valid HTTPS URL       |

### Int64

| Validator                          | Description       |
| ---------------------------------- | ----------------- |
| `int64validator.Between(min, max)` | Range (inclusive) |
| `int64validator.AtLeast(min)`      | Minimum           |
| `int64validator.AtMost(max)`       | Maximum           |
| `int64validator.OneOf(1, 2, 3)`    | Enum values       |

### Bool

| Validator                    | Description  |
| ---------------------------- | ------------ |
| `boolvalidator.Equals(true)` | Must be true |

### List/Set/Map

| Validator                             | Description           |
| ------------------------------------- | --------------------- |
| `listvalidator.SizeAtLeast(min)`      | Minimum element count |
| `listvalidator.SizeAtMost(max)`       | Maximum element count |
| `listvalidator.SizeBetween(min, max)` | Element count range   |
| `listvalidator.UniqueValues()`        | No duplicate elements |

## Conflict/Dependency Validators

Express relationships between attributes:

```go
// Exactly one of these must be set
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ExactlyOneOf(
            path.MatchRoot("field_a"),
            path.MatchRoot("field_b"),
        ),
    },
}

// At least one of these must be set
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.AtLeastOneOf(
            path.MatchRoot("field_a"),
            path.MatchRoot("field_b"),
        ),
    },
}

// These conflict (cannot both be set)
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ConflictsWith(
            path.MatchRoot("other_field"),
        ),
    },
}

// Required together (if one is set, all must be set)
schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.AlsoRequires(
            path.MatchRoot("other_field"),
        ),
    },
}
```

## Custom Validators

Implement the `validator.<Type>` interface:

```go
type emailValidator struct{}

func (v emailValidator) Description(_ context.Context) string {
    return "value must be a valid email address"
}

func (v emailValidator) MarkdownDescription(_ context.Context) string {
    return "value must be a valid email address"
}

func (v emailValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
    if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
        return
    }

    value := req.ConfigValue.ValueString()
    if !strings.Contains(value, "@") {
        resp.Diagnostics.AddAttributeError(
            req.Path,
            "Invalid Email",
            fmt.Sprintf("%q is not a valid email address", value),
        )
    }
}

// Usage:
schema.StringAttribute{
    Required:   true,
    Validators: []validator.String{emailValidator{}},
}
```

## Resource-Level Validation

For cross-attribute validation that requires access to multiple fields:

```go
var _ resource.ResourceWithValidateConfig = &fooResource{}

func (r *fooResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
    var data fooResourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // Skip validation if values are unknown (references to other resources)
    if data.FieldA.IsUnknown() || data.FieldB.IsUnknown() {
        return
    }

    // Cross-attribute validation
    if data.FieldA.ValueString() == "special" && data.FieldB.IsNull() {
        resp.Diagnostics.AddAttributeError(
            path.Root("field_b"),
            "Missing Required Field",
            "field_b is required when field_a is 'special'",
        )
    }
}
```

## Diagnostics

### Error vs Warning

```go
// Error: blocks apply
resp.Diagnostics.AddError("Title", "Detail message")

// Warning: allows apply but notifies user
resp.Diagnostics.AddWarning("Title", "Detail message")

// Attribute-specific error (shows path in output)
resp.Diagnostics.AddAttributeError(path.Root("name"), "Title", "Detail")

// Check for errors before continuing
if resp.Diagnostics.HasError() {
    return
}
```

## Related Framework References

| File                                                | Contents                      |
| --------------------------------------------------- | ----------------------------- |
| `framework/validation.mdx`                          | Full validation documentation |
| `framework/diagnostics.mdx`                         | Diagnostics (errors/warnings) |
| `framework/resources/validate-configuration.mdx`    | Resource-level validation     |
| `framework/data-sources/validate-configuration.mdx` | Data source validation        |
| `framework/providers/validate-configuration.mdx`    | Provider-level validation     |
