# Plan Modification

## Overview

After validation and before apply, Terraform generates a plan describing expected values. Plan modifiers let you:

- Provide known values for computed attributes (reduce "known after apply" noise)
- Mark resources for replacement when in-place update is impossible
- Return diagnostics on planned changes

## Plan Modification Process

1. Null config values get their default value applied
2. If plan differs from state, computed attributes with null config become unknown
3. Attribute plan modifiers run (in schema order)
4. Resource-level plan modifiers run (`ModifyPlan`)

After apply, all state values MUST match planned values or Terraform produces "Provider produced inconsistent result" error.

## Built-in Attribute Plan Modifiers

Available in `resource/schema/<type>planmodifier` packages:

### UseStateForUnknown

Copies the prior state value into the plan. Use for computed values that don't change after creation (IDs, creation timestamps).

```go
import "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

schema.StringAttribute{
    Computed: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
}
```

This provider's `rsId()` helper wraps this pattern for ID attributes.

### RequiresReplace

Forces resource destruction and recreation when the attribute value changes. Use for immutable API fields.

```go
schema.StringAttribute{
    Required: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
}
```

### RequiresReplaceIf

Conditional replacement based on provider-defined logic:

```go
stringplanmodifier.RequiresReplaceIf(
    func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
        // Only replace if changing from non-empty to different non-empty
        resp.RequiresReplace = !req.StateValue.IsNull() && !req.PlanValue.IsNull()
    },
    "Replace when changing between non-null values",
    "Replace when changing between non-null values",
)
```

### RequiresReplaceIfConfigured

Like RequiresReplace but only triggers if the practitioner explicitly configured the value (not null):

```go
stringplanmodifier.RequiresReplaceIfConfigured()
```

## Available Modifier Packages

Each type has its own package:

| Type    | Package                               | Modifiers                                                                           |
| ------- | ------------------------------------- | ----------------------------------------------------------------------------------- |
| String  | `resource/schema/stringplanmodifier`  | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| Bool    | `resource/schema/boolplanmodifier`    | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| Int64   | `resource/schema/int64planmodifier`   | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| Float64 | `resource/schema/float64planmodifier` | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| List    | `resource/schema/listplanmodifier`    | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| Map     | `resource/schema/mapplanmodifier`     | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| Set     | `resource/schema/setplanmodifier`     | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |
| Object  | `resource/schema/objectplanmodifier`  | UseStateForUnknown, RequiresReplace, RequiresReplaceIf, RequiresReplaceIfConfigured |

## Custom Plan Modifiers

Implement the relevant `planmodifier.<Type>` interface:

```go
type myModifier struct{}

func (m myModifier) Description(_ context.Context) string {
    return "Description for practitioners"
}

func (m myModifier) MarkdownDescription(_ context.Context) string {
    return "Markdown description for practitioners"
}

func (m myModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
    // Access current state, config, plan values:
    //   req.StateValue  - prior state
    //   req.ConfigValue - configuration value
    //   req.PlanValue   - current plan value
    //
    // Modify plan:
    //   resp.PlanValue = types.StringValue("new-value")
    //   resp.RequiresReplace = true
}
```

## Resource-Level Plan Modification

Implement `resource.ResourceWithModifyPlan` for cross-attribute logic:

```go
func (r *fooResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
    // Access full plan, state, config
    // Can add diagnostics, mark for replacement, modify plan values

    if req.Plan.Raw.IsNull() {
        // Resource is being destroyed
        return
    }

    // Example: warn when dangerous combination is planned
    var plan fooResourceModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if plan.DangerMode.ValueBool() && plan.Public.ValueBool() {
        resp.Diagnostics.AddWarning("Security Warning", "Enabling danger mode on a public resource")
    }
}
```

## Common Patterns in This Provider

### ID attributes (stable after creation)

```go
"id": rsId() // Uses UseStateForUnknown internally
```

### Immutable fields (force recreation)

```go
"parent_id": schema.StringAttribute{
    Required: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
}
```

### Computed with server default

```go
"org_unit_path": schema.StringAttribute{
    Optional: true,
    Computed: true, // Server assigns "/" if not provided
}
```

No UseStateForUnknown here because the value CAN change on update.

## Related Framework References

| File                                        | Contents                             |
| ------------------------------------------- | ------------------------------------ |
| `framework/resources/plan-modification.mdx` | Full plan modification documentation |
| `framework/resources/default.mdx`           | Default values (interact with plan)  |
| `framework/handling-data/schemas.mdx`       | Schema definition                    |
