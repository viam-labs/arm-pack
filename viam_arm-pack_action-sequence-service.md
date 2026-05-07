# Model viam:arm-pack:action-sequence-service

A generic service that runs an ordered sequence of high-level actions against
configured `gripper` and `switch` components on the same machine. Each action
is one of:

- `grab` — call `Grab` on a configured gripper.
- `open` — call `Open` on a configured gripper.
- `move_position` — drive a configured switch (used here as a "saved position"
  selector) to position `2` via `SetPosition`.

The action list is fully data-driven via the `actions` config field, and the
sequence is triggered at runtime via `DoCommand`.

## Configuration

The following attribute template can be used to configure this model:

```json
{
  "actions": [
    {
      "action": "<grab|open|move_position>",
      "params": {
        "gripper": "<string, for grab/open>",
        "saved_position": "<string, for move_position>"
      }
    }
  ]
}
```

### Attributes

The following attributes are available for this model:

| Name      | Type             | Inclusion | Description                                                                 |
|-----------|------------------|-----------|-----------------------------------------------------------------------------|
| `actions` | array of objects | Required  | Ordered list of actions to execute. Must contain at least one action.       |

Each entry in `actions` has the following fields:

| Name             | Type   | Inclusion                              | Description                                                                                         |
|------------------|--------|----------------------------------------|-----------------------------------------------------------------------------------------------------|
| `action`         | string | Required                               | One of `"grab"`, `"open"`, or `"move_position"`.                                                    |
| `params.gripper` | string | Required for `grab` / `open`           | Name of a configured `gripper` component on the same machine. Added as a required dependency.       |
| `params.saved_position` | string | Required for `move_position`     | Name of a configured `switch` component representing a saved position. Added as a required dependency. |

Validation rules:

- `actions` must be non-empty.
- `grab` and `open` require `params.gripper` and must not set `params.saved_position`.
- `move_position` requires `params.saved_position` and must not set `params.gripper`.
- Every `gripper` and `saved_position` referenced must be the name of an
  existing component on the machine; the service declares them as dependencies
  and resolves them at construction time.

### Example Configuration

```json
{
  "actions": [
    { "action": "open",          "params": { "gripper": "my-gripper" } },
    { "action": "move_position", "params": { "saved_position": "above-bin" } },
    { "action": "grab",          "params": { "gripper": "my-gripper" } },
    { "action": "move_position", "params": { "saved_position": "drop-zone" } },
    { "action": "open",          "params": { "gripper": "my-gripper" } }
  ]
}
```

## DoCommand

`DoCommand` accepts a single field, `command`. The only supported value today
is `"execute"`, which runs the configured `actions` array in order.

When `command` is `"execute"`:

- Each `grab` action calls `Grab` on the named gripper.
- Each `open` action calls `Open` on the named gripper.
- Each `move_position` action calls `SetPosition(2, nil)` on the named switch.
- If any action returns an error, execution stops and the error is returned,
  wrapped with the failing action's index and type.
- The context is checked between actions, so an in-flight sequence can be
  cancelled by the caller.

On success, `DoCommand` returns:

```json
{ "status": "ok" }
```

### Example DoCommand

```json
{
  "command": "execute"
}
```
