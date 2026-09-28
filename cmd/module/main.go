package main

import (
	"armpack"
	arm "go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/module"
	"go.viam.com/rdk/resource"
	generic "go.viam.com/rdk/services/generic"
)

func main() {
	module.ModularMain(
		resource.APIModel{API: generic.API, Model: armpack.ActionSequenceService},
		resource.APIModel{API: arm.API, Model: armpack.DialArmControl},
	)
}
