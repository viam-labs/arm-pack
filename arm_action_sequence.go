package armpack

import (
	"context"
	"errors"
	"fmt"

	"go.viam.com/rdk/components/gripper"
	toggleswitch "go.viam.com/rdk/components/switch"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
	generic "go.viam.com/rdk/services/generic"
)

var (
	ActionSequenceService = resource.NewModel("viam", "arm-pack", "action-sequence-service")
	errUnimplemented      = errors.New("unimplemented")
)

const (
	ActionGrab         = "grab"
	ActionOpen         = "open"
	ActionMovePosition = "move_position"
)

const (
	doCommandKey     = "command"
	doCommandExecute = "execute"

	movePositionTarget uint32 = 2
)

func init() {
	resource.RegisterService(generic.API, ActionSequenceService,
		resource.Registration[resource.Resource, *Config]{
			Constructor: newArmPackActionSequenceService,
		},
	)
}

// ActionParams holds the parameters for a single action. Which fields
// are required depends on the action type:
//   - "grab" and "open" require Gripper.
//   - "move_position" requires SavedPosition.
type ActionParams struct {
	Gripper       string `json:"gripper,omitempty"`
	SavedPosition string `json:"saved_position,omitempty"`
}

// Action is a single step in the action sequence.
type Action struct {
	Action string       `json:"action"`
	Params ActionParams `json:"params"`
}

type Config struct {
	Actions []Action `json:"actions"`
}

// Validate ensures all parts of the config are valid and important fields exist.
// Returns three values:
//  1. Required dependencies: other resources that must exist for this resource to work.
//  2. Optional dependencies: other resources that may exist but are not required.
//  3. An error if any Config fields are missing or invalid.
//
// The `path` parameter indicates
// where this resource appears in the machine's JSON configuration
// (for example, "components.0"). You can use it in error messages
// to indicate which resource has a problem.
func (cfg *Config) Validate(path string) ([]string, []string, error) {
	if len(cfg.Actions) == 0 {
		return nil, nil, fmt.Errorf(`%s: "actions" must contain at least one action`, path)
	}

	var deps []string
	seen := map[string]struct{}{}
	addDep := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		deps = append(deps, name)
	}

	for i, action := range cfg.Actions {
		actionPath := fmt.Sprintf("%s.actions.%d", path, i)
		switch action.Action {
		case ActionGrab, ActionOpen:
			if action.Params.Gripper == "" {
				return nil, nil, fmt.Errorf(`%s: %q action requires "gripper" param`, actionPath, action.Action)
			}
			if action.Params.SavedPosition != "" {
				return nil, nil, fmt.Errorf(`%s: %q action does not support "saved_position" param`, actionPath, action.Action)
			}
			addDep(action.Params.Gripper)
		case ActionMovePosition:
			if action.Params.SavedPosition == "" {
				return nil, nil, fmt.Errorf(`%s: %q action requires "saved_position" param`, actionPath, action.Action)
			}
			if action.Params.Gripper != "" {
				return nil, nil, fmt.Errorf(`%s: %q action does not support "gripper" param`, actionPath, action.Action)
			}
			addDep(action.Params.SavedPosition)
		case "":
			return nil, nil, fmt.Errorf(`%s: "action" field is required`, actionPath)
		default:
			return nil, nil, fmt.Errorf(`%s: unsupported action %q (must be one of %q, %q, %q)`,
				actionPath, action.Action, ActionGrab, ActionOpen, ActionMovePosition)
		}
	}

	return deps, nil, nil
}

type armPackActionSequenceService struct {
	resource.AlwaysRebuild
	resource.Named

	name resource.Name

	logger logging.Logger
	cfg    *Config

	grippers map[string]gripper.Gripper
	switches map[string]toggleswitch.Switch

	cancelCtx  context.Context
	cancelFunc func()
}

func newArmPackActionSequenceService(ctx context.Context, deps resource.Dependencies, rawConf resource.Config, logger logging.Logger) (resource.Resource, error) {
	conf, err := resource.NativeConfig[*Config](rawConf)
	if err != nil {
		return nil, err
	}

	return NewActionSequenceService(ctx, deps, rawConf.ResourceName(), conf, logger)

}

func NewActionSequenceService(ctx context.Context, deps resource.Dependencies, name resource.Name, conf *Config, logger logging.Logger) (resource.Resource, error) {

	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	grippers := map[string]gripper.Gripper{}
	switches := map[string]toggleswitch.Switch{}

	for _, action := range conf.Actions {
		switch action.Action {
		case ActionGrab, ActionOpen:
			gripperName := action.Params.Gripper
			if _, ok := grippers[gripperName]; ok {
				continue
			}
			g, err := gripper.FromDependencies(deps, gripperName)
			if err != nil {
				cancelFunc()
				return nil, fmt.Errorf("could not get gripper %q from dependencies: %w", gripperName, err)
			}
			grippers[gripperName] = g
		case ActionMovePosition:
			switchName := action.Params.SavedPosition
			if _, ok := switches[switchName]; ok {
				continue
			}
			sw, err := toggleswitch.FromDependencies(deps, switchName)
			if err != nil {
				cancelFunc()
				return nil, fmt.Errorf("could not get switch %q from dependencies: %w", switchName, err)
			}
			switches[switchName] = sw
		}
	}

	s := &armPackActionSequenceService{
		name:       name,
		logger:     logger,
		cfg:        conf,
		grippers:   grippers,
		switches:   switches,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}
	return s, nil
}

func (s *armPackActionSequenceService) Name() resource.Name {
	return s.name
}

func (s *armPackActionSequenceService) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	rawCommand, ok := cmd[doCommandKey]
	if !ok {
		return nil, fmt.Errorf("missing required %q field", doCommandKey)
	}
	command, ok := rawCommand.(string)
	if !ok {
		return nil, fmt.Errorf("%q must be a string, got %T", doCommandKey, rawCommand)
	}

	switch command {
	case doCommandExecute:
		if err := s.executeSequence(ctx); err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "ok"}, nil
	default:
		return nil, fmt.Errorf("unsupported command %q", command)
	}
}

// executeSequence runs every configured action in order.
func (s *armPackActionSequenceService) executeSequence(ctx context.Context) error {
	for i, action := range s.cfg.Actions {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("action sequence cancelled at action %d: %w", i, err)
		}
		if err := s.runAction(ctx, action); err != nil {
			return fmt.Errorf("action %d (%s) failed: %w", i, action.Action, err)
		}
	}
	return nil
}

func (s *armPackActionSequenceService) runAction(ctx context.Context, action Action) error {
	switch action.Action {
	case ActionGrab:
		g, ok := s.grippers[action.Params.Gripper]
		if !ok {
			return fmt.Errorf("gripper %q not found", action.Params.Gripper)
		}
		if _, err := g.Grab(ctx, nil); err != nil {
			return err
		}
		return nil
	case ActionOpen:
		g, ok := s.grippers[action.Params.Gripper]
		if !ok {
			return fmt.Errorf("gripper %q not found", action.Params.Gripper)
		}
		return g.Open(ctx, nil)
	case ActionMovePosition:
		sw, ok := s.switches[action.Params.SavedPosition]
		if !ok {
			return fmt.Errorf("saved position switch %q not found", action.Params.SavedPosition)
		}
		return sw.SetPosition(ctx, movePositionTarget, nil)
	default:
		return fmt.Errorf("unsupported action %q", action.Action)
	}
}

func (s *armPackActionSequenceService) Status(ctx context.Context) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *armPackActionSequenceService) Close(context.Context) error {
	// Put close code here
	s.cancelFunc()
	return nil
}
