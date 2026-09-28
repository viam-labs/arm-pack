package armpack

import (
	"context"
	"fmt"

	commonpb "go.viam.com/api/common/v1"
	arm "go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/spatialmath"
)

var DialArmControl = resource.NewModel("viam", "arm-pack", "dial-arm-control")

func init() {
	resource.RegisterComponent(arm.API, DialArmControl,
		resource.Registration[arm.Arm, *DialArmControlConfig]{
			Constructor: newDialArmControl,
		},
	)
}

type DialArmControlConfig struct {
	Arm             string  `json:"arm"`
	DialMoveXMM     float64 `json:"dial_move_x_mm,omitempty"`
	DialMoveYMM     float64 `json:"dial_move_y_mm,omitempty"`
	DialMoveZMM     float64 `json:"dial_move_z_mm,omitempty"`
	DialMaxPosition float64 `json:"dial_max_position,omitempty"`
}

func (cfg *DialArmControlConfig) Validate(path string) ([]string, []string, error) {
	if cfg.Arm == "" {
		return nil, nil, fmt.Errorf("arm is required")
	}
	return []string{cfg.Arm}, nil, nil
}

type dialArmControl struct {
	resource.AlwaysRebuild

	name resource.Name

	logger logging.Logger
	cfg    *DialArmControlConfig
	arm    arm.Arm

	lastDialX    *float64
	lastDialY    *float64
	lastDialZ    *float64
	lastDialDirX float64
	lastDialDirY float64
	lastDialDirZ float64

	cancelCtx  context.Context
	cancelFunc func()
}

func newDialArmControl(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (arm.Arm, error) {
	conf, err := resource.NativeConfig[*DialArmControlConfig](rawConf)
	if err != nil {
		return nil, err
	}

	return NewDialArmControl(ctx, deps, rawConf.ResourceName(), conf, logger)

}

func NewDialArmControl(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *DialArmControlConfig, logger logging.Logger) (arm.Arm, error) {
	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	s := &dialArmControl{
		name:       name,
		logger:     logger,
		cfg:        conf,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}

	armRes, err := arm.FromDependencies(deps, conf.Arm)
	if err != nil {
		cancelFunc()
		return nil, fmt.Errorf("failed to get arm dependency %q: %w", conf.Arm, err)
	}
	s.arm = armRes

	return s, nil
}

func (s *dialArmControl) Name() resource.Name {
	return s.name
}

// EndPosition returns the current position of the arm.
func (s *dialArmControl) EndPosition(ctx context.Context, extra map[string]interface{}) (spatialmath.Pose, error) {
	return s.arm.EndPosition(ctx, extra)
}

// MoveToPosition moves the arm to the given absolute position.
// This will block until done or a new operation cancels this one.
func (s *dialArmControl) MoveToPosition(ctx context.Context, pose spatialmath.Pose, extra map[string]interface{}) error {
	return s.arm.MoveToPosition(ctx, pose, extra)
}

// MoveToJointPositions moves the arm's joints to the given positions.
// This will block until done or a new operation cancels this one.
func (s *dialArmControl) MoveToJointPositions(ctx context.Context, positions []referenceframe.Input, extra map[string]interface{}) error {
	return s.arm.MoveToJointPositions(ctx, positions, extra)
}

// MoveThroughJointPositions moves the arm's joints through the given positions in the order they are specified.
// This will block until done or a new operation cancels this one.
func (s *dialArmControl) MoveThroughJointPositions(ctx context.Context, positions [][]referenceframe.Input, options *arm.MoveOptions, extra map[string]interface{}) error {
	return s.arm.MoveThroughJointPositions(ctx, positions, options, extra)
}

// MoveThroughJointPositionsStreamed forwards a stream of trajectory point batches to the underlying arm.
func (s *dialArmControl) MoveThroughJointPositionsStreamed(
	ctx context.Context,
	batches <-chan []arm.TrajectoryPoint,
	responses chan<- arm.Response,
	extra map[string]interface{},
) error {
	return s.arm.MoveThroughJointPositionsStreamed(ctx, batches, responses, extra)
}

// JointPositions returns the current joint positions of the arm.
func (s *dialArmControl) JointPositions(ctx context.Context, extra map[string]interface{}) ([]referenceframe.Input, error) {
	return s.arm.JointPositions(ctx, extra)
}

func (s *dialArmControl) Stop(ctx context.Context, extra map[string]interface{}) error {
	return s.arm.Stop(ctx, extra)
}

func (s *dialArmControl) Kinematics(ctx context.Context) (referenceframe.Model, error) {
	return s.arm.Kinematics(ctx)
}

func (s *dialArmControl) CurrentInputs(ctx context.Context) ([]referenceframe.Input, error) {
	return s.arm.CurrentInputs(ctx)
}

func (s *dialArmControl) GoToInputs(ctx context.Context, inputSteps ...[]referenceframe.Input) error {
	return s.arm.GoToInputs(ctx, inputSteps...)
}

func (s *dialArmControl) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	s.logger.Infof("DoCommand called with args: %+v", cmd)

	if v, ok := cmd["dial_move_x"]; ok {
		return s.handleDialMove(ctx, "x", v)
	}
	if v, ok := cmd["dial_move_y"]; ok {
		return s.handleDialMove(ctx, "y", v)
	}
	if v, ok := cmd["dial_move_z"]; ok {
		return s.handleDialMove(ctx, "z", v)
	}

	return nil, fmt.Errorf("unknown command: %v", cmd)
}

func (s *dialArmControl) handleDialMove(ctx context.Context, axis string, dialValue interface{}) (map[string]interface{}, error) {
	var mm float64
	switch axis {
	case "x":
		mm = s.cfg.DialMoveXMM
	case "y":
		mm = s.cfg.DialMoveYMM
	case "z":
		mm = s.cfg.DialMoveZMM
	}
	if mm == 0 {
		mm = 1
	}

	if dialVal, ok := toFloat64(dialValue); ok {
		var last **float64
		var lastDir *float64
		switch axis {
		case "x":
			last = &s.lastDialX
			lastDir = &s.lastDialDirX
		case "y":
			last = &s.lastDialY
			lastDir = &s.lastDialDirY
		case "z":
			last = &s.lastDialZ
			lastDir = &s.lastDialDirZ
		}
		if *last == nil {
			*last = &dialVal
			return map[string]interface{}{"status": "dial_initialized", "axis": axis, "position": dialVal}, nil
		}
		maxPos := s.cfg.DialMaxPosition
		if maxPos == 0 {
			maxPos = 100
		}
		delta := dialVal - **last
		if delta > maxPos/2 {
			delta -= maxPos + 1
		} else if delta < -maxPos/2 {
			delta += maxPos + 1
		}
		var direction float64
		if **last == 0 && *lastDir != 0 {
			direction = *lastDir
		} else if delta < 0 {
			direction = -1
		} else {
			direction = 1
		}
		if direction < 0 {
			mm = -mm
		}
		*lastDir = direction
		**last = dialVal
	}

	currentPose, err := s.arm.EndPosition(ctx, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("failed to get current arm position: %w", err)
	}
	pt := currentPose.Point()
	switch axis {
	case "x":
		pt.X += mm
	case "y":
		pt.Y += mm
	case "z":
		pt.Z += mm
	default:
		return nil, fmt.Errorf("invalid axis %q; must be 'x', 'y', or 'z'", axis)
	}
	newPose := spatialmath.NewPose(pt, currentPose.Orientation())
	if err := s.arm.MoveToPosition(ctx, newPose, map[string]interface{}{}); err != nil {
		return nil, fmt.Errorf("failed to move arm: %w", err)
	}
	return map[string]interface{}{"status": "moved", "axis": axis, "mm": mm}, nil
}

func (s *dialArmControl) IsMoving(ctx context.Context) (bool, error) {
	return s.arm.IsMoving(ctx)
}

func (s *dialArmControl) Geometries(ctx context.Context, extra map[string]interface{}) ([]spatialmath.Geometry, error) {
	return s.arm.Geometries(ctx, extra)
}

// Get3DModels returns the 3D models of the arm.
func (s *dialArmControl) Get3DModels(ctx context.Context, extra map[string]interface{}) (map[string]*commonpb.Mesh, error) {
	return s.arm.Get3DModels(ctx, extra)
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func (s *dialArmControl) Status(ctx context.Context) (map[string]interface{}, error) {
	return s.arm.Status(ctx)
}

func (s *dialArmControl) Close(context.Context) error {
	s.cancelFunc()
	return nil
}
