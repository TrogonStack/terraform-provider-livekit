# Validation

## Overview

Validation runs during `terraform validate`, `terraform plan`, and `terraform apply`. It returns diagnostics (warnings/errors) before any API calls happen. Validation occurs at two levels:

1. **Attribute-level validators**: validate individual attribute values
2. **Resource/data-source-level validation**: cross-attribute validation logic

Important: configuration values may be unknown during validation (references to other resources). Validators must handle this by returning early without diagnostics.

## Attribute Validators

Add validators to any attribute's `Validators` field. All validators in the slice always run (no short-circuit).

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/schema/validator"
    "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

schema.StringAttribute{
    Optional: true,
    Computed: true,
    Default:  stringdefault.StaticString(string(secretKindEnvironment)),
    MarkdownDescription: "One of `environment` or `file`. Defaults to `environment`.",
    Validators: []validator.String{
        stringvalidator.OneOf(string(secretKindEnvironment), string(secretKindFile)),
    },
}
```

This is the actual `kind` attribute on `livekit_agent_secret` (`resource_agent_secret.go`), the only validator currently used anywhere in this provider. The `stringvalidator.OneOf` constraint mirrors what `parseSecretKind` in `secret_kind.go` accepts at apply time; the validator gives the practitioner the same error earlier, at plan time.

## Common Validators (terraform-plugin-framework-validators)

### String

| Validator                                     | Description           |
| ------------------------------------------------- | ------------------------ |
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
| -------------------------------------- | -------------------- |
| `int64validator.Between(min, max)` | Range (inclusive) |
| `int64validator.AtLeast(min)`      | Minimum           |
| `int64validator.AtMost(max)`       | Maximum           |
| `int64validator.OneOf(1, 2, 3)`    | Enum values       |

### Bool

| Validator                    | Description  |
| --------------------------------- | -------------- |
| `boolvalidator.Equals(true)` | Must be true |

### List/Set/Map

| Validator                             | Description           |
| ------------------------------------------ | ------------------------ |
| `listvalidator.SizeAtLeast(min)`      | Minimum element count |
| `listvalidator.SizeAtMost(max)`       | Maximum element count |
| `listvalidator.SizeBetween(min, max)` | Element count range   |
| `listvalidator.UniqueValues()`        | No duplicate elements |

`setvalidator` has the same shapes for `livekit_agent.regions`, though this provider does not currently constrain it (any region string LiveKit Cloud accepts is passed through as-is).

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

Not used anywhere in this provider today; both resource schemas are small enough that no attribute's validity depends on another's.

## Custom Validators

Implement the `validator.<Type>` interface. This provider has no custom validators today (`stringvalidator.OneOf` above covers its one constrained attribute); the shape below is illustrative:

```go
type secretNameValidator struct{}

func (v secretNameValidator) Description(_ context.Context) string {
    return "value must be a non-empty secret name"
}

func (v secretNameValidator) MarkdownDescription(_ context.Context) string {
    return "value must be a non-empty secret name"
}

func (v secretNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
    if req.ConfigValue.IsUnknown() || req.ConfigValue.IsNull() {
        return
    }

    value := req.ConfigValue.ValueString()
    if strings.TrimSpace(value) == "" {
        resp.Diagnostics.AddAttributeError(
            req.Path,
            "Invalid Secret Name",
            fmt.Sprintf("%q is not a valid secret name", value),
        )
    }
}

// Usage:
schema.StringAttribute{
    Required:   true,
    Validators: []validator.String{secretNameValidator{}},
}
```

## Resource-Level Validation

For cross-attribute validation that requires access to multiple fields. Not used anywhere in this provider today (neither `agentResource` nor `agentSecretResource` implements `resource.ResourceWithValidateConfig`); the shape below is illustrative:

```go
var _ resource.ResourceWithValidateConfig = &agentSecretResource{}

func (r *agentSecretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
    var data agentSecretResourceModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // Skip validation if values are unknown (references to other resources)
    if data.Kind.IsUnknown() || data.Name.IsUnknown() {
        return
    }

    if data.Kind.ValueString() == "file" && data.Name.ValueString() == "" {
        resp.Diagnostics.AddAttributeError(
            path.Root("name"),
            "Missing Required Field",
            "name is required when kind is 'file'",
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

Both resources use `resp.Diagnostics.AddError("API Error", ...)` and `resp.Diagnostics.AddError("Invalid Configuration", ...)` (for a `parseSecretKind` failure) as their primary error-reporting pattern; see `references/guides/resource-lifecycle.md`.

## Related Framework References

| File                                                | Contents                      |
| -------------------------------------------------------- | -------------------------------- |
| `framework/validation.mdx`                          | Full validation documentation |
| `framework/diagnostics.mdx`                         | Diagnostics (errors/warnings) |
| `framework/resources/validate-configuration.mdx`    | Resource-level validation     |
| `framework/data-sources/validate-configuration.mdx` | Data source validation        |
| `framework/providers/validate-configuration.mdx`    | Provider-level validation     |
