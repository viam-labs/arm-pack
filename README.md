# Module arm-pack

A collection of models for building robot arm applications on Viam. It
includes a service for scripting pick-and-place style action sequences and a
service for jogging an arm with dial input.

## Models

This module provides the following models:

| Model                                                                                 | API                   | Description                                                                                                                                                            |
| ------------------------------------------------------------------------------------- | --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`viam:arm-pack:action-sequence-service`](#model-viamarm-packaction-sequence-service) | `rdk:service:generic` | Runs a configured, ordered list of gripper `grab`/`open` and saved-position `move_position` actions when triggered with `DoCommand`.                                   |
| [`viam:arm-pack:dial-control-motion`](#model-viamarm-packdial-control-motion)         | `rdk:service:generic` | Translates dial input into relative end-effector translations and rotations of a configured arm, with speed-based acceleration and a translation/rotation mode toggle. |

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

## Model: `viam:arm-pack:dial-control-motion`

A generic service that turns dial input (for example a Stream Deck dial) into
relative motion of a configured arm's end effector. It can translate the end
effector along the base frame's X, Y and Z axes or along the end effector's
current pointing direction, and rotate it in place around its own local X, Y
and Z axes.

The dial reports absolute positions through `DoCommand`. Each call infers a
direction from the change since the previous reading and queues one signed
step. A background loop flushes the queued steps to the arm every
`drain_interval_ms` in a single `MoveToPosition` call, so detents that arrive
between flushes are combined. Turning the dial faster raises a per-axis
acceleration multiplier, so quick spins cover more distance per detent than
slow, precise turns.

### Configuration

The following attribute template can be used to configure this model:

```json
{
  "arm_name": "<string>",
  "dial_move_x_mm": <float>,
  "dial_move_y_mm": <float>,
  "dial_move_z_mm": <float>,
  "dial_move_orientation_mm": <float>,
  "dial_move_rx_deg": <float>,
  "dial_move_ry_deg": <float>,
  "dial_move_rz_deg": <float>,
  "dial_max_position": <float>,
  "drain_interval_ms": <int>,
  "accel_threshold_count": <float>,
  "accel_max_multiplier": <float>,
  "accel_exponent": <float>,
  "accel_smoothing_alpha": <float>,
  "accel_rotation_threshold_count": <float>,
  "accel_rotation_max_multiplier": <float>,
  "accel_rotation_exponent": <float>,
  "accel_rotation_smoothing_alpha": <float>
}
```

#### Attributes

The following attributes are available for this model:

| Name                             | Type   | Inclusion | Description                                                                                                                                   |
| -------------------------------- | ------ | --------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `arm_name`                       | string | Required  | Name of the configured `arm` component to move. Added as a required dependency.                                                               |
| `dial_move_x_mm`                 | float  | Optional  | Millimeters per detent along the base frame X axis. Defaults to `1`.                                                                          |
| `dial_move_y_mm`                 | float  | Optional  | Millimeters per detent along the base frame Y axis. Defaults to `1`.                                                                          |
| `dial_move_z_mm`                 | float  | Optional  | Millimeters per detent along the base frame Z axis. Defaults to `1`.                                                                          |
| `dial_move_orientation_mm`       | float  | Optional  | Millimeters per detent along the end effector's current orientation vector (moving "forward/back" along the tool). Defaults to `1`.           |
| `dial_move_rx_deg`               | float  | Optional  | Degrees per detent around the end effector's local X axis. Defaults to `1`.                                                                   |
| `dial_move_ry_deg`               | float  | Optional  | Degrees per detent around the end effector's local Y axis. Defaults to `1`.                                                                   |
| `dial_move_rz_deg`               | float  | Optional  | Degrees per detent around the end effector's local Z axis. Defaults to `1`.                                                                   |
| `dial_max_position`              | float  | Optional  | Highest value the dial reports before wrapping back to `0`. Used to detect wraparound and saturation. Defaults to `100`.                      |
| `drain_interval_ms`              | int    | Optional  | How often queued steps are flushed to the arm, in milliseconds. Defaults to `20` (50 Hz).                                                     |
| `accel_threshold_count`          | float  | Optional  | Smoothed detents per drain window at which acceleration starts. Below it the multiplier is `1`. Defaults to `1`.                              |
| `accel_max_multiplier`           | float  | Optional  | Upper bound on the acceleration multiplier. Defaults to `10`.                                                                                 |
| `accel_exponent`                 | float  | Optional  | Exponent of the acceleration curve; higher values ramp up more sharply. Defaults to `1.5`.                                                    |
| `accel_smoothing_alpha`          | float  | Optional  | EWMA smoothing factor in `(0, 1]` for the detent rate. `1` reacts instantly; smaller values ramp and decay more gradually. Defaults to `0.4`. |
| `accel_rotation_threshold_count` | float  | Optional  | Override of `accel_threshold_count` for rotation axes. Falls back to the translation value.                                                   |
| `accel_rotation_max_multiplier`  | float  | Optional  | Override of `accel_max_multiplier` for rotation axes. Falls back to the translation value.                                                    |
| `accel_rotation_exponent`        | float  | Optional  | Override of `accel_exponent` for rotation axes. Falls back to the translation value.                                                          |
| `accel_rotation_smoothing_alpha` | float  | Optional  | Override of `accel_smoothing_alpha` for rotation axes. Falls back to the translation value.                                                   |

The acceleration multiplier for an axis is computed every drain window as:

```
smoothed   = alpha * detents_this_window + (1 - alpha) * smoothed_previous
multiplier = clamp((smoothed / threshold) ^ exponent, 1, max_multiplier)
```

#### Example Configuration

```json
{
  "arm_name": "my-arm",
  "dial_move_x_mm": 2,
  "dial_move_y_mm": 2,
  "dial_move_z_mm": 1,
  "dial_move_rx_deg": 2,
  "dial_move_ry_deg": 2,
  "dial_move_rz_deg": 2,
  "accel_max_multiplier": 8
}
```

### DoCommand

#### Dial moves

`DoCommand` accepts one of the following keys, each with the dial's current
absolute position as a number:

| Command                 | Motion                                                          |
| ----------------------- | --------------------------------------------------------------- |
| `dial_move_x`           | Translate along base X (rotate around local X in rotation mode) |
| `dial_move_y`           | Translate along base Y (rotate around local Y in rotation mode) |
| `dial_move_z`           | Translate along base Z (rotate around local Z in rotation mode) |
| `dial_move_orientation` | Translate along the end effector's orientation vector           |
| `dial_move_rx`          | Rotate around the end effector's local X axis                   |
| `dial_move_ry`          | Rotate around the end effector's local Y axis                   |
| `dial_move_rz`          | Rotate around the end effector's local Z axis                   |

How each reading is interpreted:

- The first reading for an axis only records the position and does not move
  the arm.
- A later reading queues one step in the positive direction if the dial turned
  up, or the negative direction if it turned down.
- A jump larger than half of `dial_max_position` is treated as the dial
  wrapping around (for example `98 → 1` counts as turning up).
- Repeating the same value is a no-op, unless the dial is pinned at `0` or
  `dial_max_position` while still being turned in that direction; then motion
  continues in the last direction.

Example:

```json
{ "dial_move_x": 42 }
```

Responses:

```json
{ "status": "dial_initialized", "axis": "x", "position": 42 }
{ "status": "queued", "axis": "x", "step": -2 }
{ "status": "no_change", "axis": "x", "position": 42 }
```

`step` is the signed base step (mm or degrees) before acceleration. The arm
moves asynchronously on the next drain tick; movement errors are logged rather
than returned.

#### Axis mode

In `rotation` mode, `dial_move_x`, `dial_move_y` and `dial_move_z` are routed
to `rx`, `ry` and `rz`, so the same three dials can switch between translating
and rotating. `dial_move_orientation` and the explicit `rx`/`ry`/`rz` commands
are unaffected. The service starts in `translation` mode.

| Command                              | Response                                           |
| ------------------------------------ | -------------------------------------------------- |
| `{ "toggle_axis_mode": true }`       | `{ "status": "toggled", "axis_mode": "rotation" }` |
| `{ "set_axis_mode": "translation" }` | `{ "status": "set", "axis_mode": "translation" }`  |
| `{ "get_axis_mode": true }`          | `{ "axis_mode": "translation" }`                   |

Any other command returns an `unknown command` error.
