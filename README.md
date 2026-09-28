# Module arm-pack

A collection of models for building robot arm applications on Viam. It
includes a service for scripting pick-and-place style action sequences and an
arm wrapper for jogging an arm with a dial input.

## Models

This module provides the following models:

| Model                                                                                 | API                   | Description                                                                                                                                                                       |
| ------------------------------------------------------------------------------------- | --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`viam:arm-pack:action-sequence-service`](#model-viamarm-packaction-sequence-service) | `rdk:service:generic` | Runs a configured, ordered list of gripper `grab`/`open` and saved-position `move_position` actions when triggered with `DoCommand`.                                              |
| [`viam:arm-pack:dial-arm-control`](#model-viamarm-packdial-arm-control)               | `rdk:component:arm`   | Wraps an existing arm, forwarding all arm API calls, and adds `DoCommand` commands to jog the end effector along X, Y or Z in fixed millimeter steps driven by a dial's position. |

## Model: `viam:arm-pack:action-sequence-service`

A generic service that runs an ordered sequence of high-level actions against
configured `gripper` and `switch` components on the same machine. Each action
is one of:

- `grab` — call `Grab` on a configured gripper.
- `open` — call `Open` on a configured gripper.
- `move_position` — drive a configured switch (used here as a "saved position"
  selector) to position `2` via `SetPosition`.

The action list is fully data-driven via the `actions` config field, and the
sequence is triggered at runtime via `DoCommand`.

### Configuration

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

#### Attributes

The following attributes are available for this model:

| Name      | Type             | Inclusion | Description                                                           |
| --------- | ---------------- | --------- | --------------------------------------------------------------------- |
| `actions` | array of objects | Required  | Ordered list of actions to execute. Must contain at least one action. |

Each entry in `actions` has the following fields:

| Name                    | Type   | Inclusion                    | Description                                                                                            |
| ----------------------- | ------ | ---------------------------- | ------------------------------------------------------------------------------------------------------ |
| `action`                | string | Required                     | One of `"grab"`, `"open"`, or `"move_position"`.                                                       |
| `params.gripper`        | string | Required for `grab` / `open` | Name of a configured `gripper` component on the same machine. Added as a required dependency.          |
| `params.saved_position` | string | Required for `move_position` | Name of a configured `switch` component representing a saved position. Added as a required dependency. |

Validation rules:

- `actions` must be non-empty.
- `grab` and `open` require `params.gripper` and must not set `params.saved_position`.
- `move_position` requires `params.saved_position` and must not set `params.gripper`.
- Every `gripper` and `saved_position` referenced must be the name of an
  existing component on the machine; the service declares them as dependencies
  and resolves them at construction time.

#### Example Configuration

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

### DoCommand

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

#### Example DoCommand

```json
{
  "command": "execute"
}
```

## Model: `viam:arm-pack:dial-arm-control`

An `arm` component that wraps another configured arm and adds dial-driven
Cartesian jogging. Every standard arm API call (`MoveToPosition`,
`JointPositions`, `Stop`, `Kinematics`, …) is forwarded unchanged to the
underlying arm, so this model can be used anywhere a regular arm is expected.

On top of that, `DoCommand` accepts `dial_move_x`, `dial_move_y` and
`dial_move_z` commands. Each one nudges the arm's end effector a fixed number
of millimeters along the matching axis of the arm's base frame while keeping its
current orientation. This makes it easy to drive an arm from a rotary encoder,
knob or other "dial" input.

### Configuration

The following attribute template can be used to configure this model:

```json
{
  "arm": "<string>",
  "dial_move_x_mm": <float>,
  "dial_move_y_mm": <float>,
  "dial_move_z_mm": <float>,
  "dial_max_position": <float>
}
```

#### Attributes

The following attributes are available for this model:

| Name                | Type   | Inclusion | Description                                                                                               |
| ------------------- | ------ | --------- | --------------------------------------------------------------------------------------------------------- |
| `arm`               | string | Required  | Name of the configured `arm` component to wrap. Added as a required dependency.                           |
| `dial_move_x_mm`    | float  | Optional  | Step size in millimeters for each `dial_move_x` command. Defaults to `1`.                                 |
| `dial_move_y_mm`    | float  | Optional  | Step size in millimeters for each `dial_move_y` command. Defaults to `1`.                                 |
| `dial_move_z_mm`    | float  | Optional  | Step size in millimeters for each `dial_move_z` command. Defaults to `1`.                                 |
| `dial_max_position` | float  | Optional  | Highest value the dial reports before wrapping back to `0`. Used to detect wraparound. Defaults to `100`. |

#### Example Configuration

```json
{
  "arm": "my-arm",
  "dial_move_x_mm": 5,
  "dial_move_y_mm": 5,
  "dial_move_z_mm": 2,
  "dial_max_position": 100
}
```

### DoCommand

`DoCommand` accepts exactly one of `dial_move_x`, `dial_move_y` or
`dial_move_z`. The value controls how the direction of the move is chosen:

- **Numeric value (dial position)** — the value is treated as the dial's
  current absolute position. The first command for an axis only records the
  position and does not move the arm. Each later command compares the new
  position to the previous one and moves the arm one step in the positive
  direction if the dial turned up, or the negative direction if it turned down.
  A jump larger than half of `dial_max_position` is treated as the dial
  wrapping around (for example `100 → 0` counts as turning up).
- **Non-numeric value (for example `true`)** — the arm moves one step in the
  positive direction along that axis.

The step size comes from the matching `dial_move_<axis>_mm` attribute. Each
axis tracks its dial position independently.

#### Example DoCommand

Report a dial position of `42` for the X axis:

```json
{
  "dial_move_x": 42
}
```

Move one step in +Z:

```json
{
  "dial_move_z": true
}
```

#### Responses

When a dial position is recorded for the first time:

```json
{ "status": "dial_initialized", "axis": "x", "position": 42 }
```

After a move (`mm` is negative when the arm moved in the negative direction):

```json
{ "status": "moved", "axis": "x", "mm": -5 }
```

Any other command returns an `unknown command` error.
