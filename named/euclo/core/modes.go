package core

import "codeburg.org/lexbit/relurpify/named/euclo/euclotypes"

type ModeDescriptor = euclotypes.ModeDescriptor
type ModeRegistry = euclotypes.ModeRegistry
type ModeResolution = euclotypes.ModeResolution
type ExecutionProfileDescriptor = euclotypes.ExecutionProfileDescriptor
type ExecutionProfileRegistry = euclotypes.ExecutionProfileRegistry
type ExecutionProfileSelection = euclotypes.ExecutionProfileSelection
type CapabilitySnapshot = euclotypes.CapabilitySnapshot

var NewModeRegistry = euclotypes.NewModeRegistry
var DefaultModeRegistry = euclotypes.DefaultModeRegistry
var NewExecutionProfileRegistry = euclotypes.NewExecutionProfileRegistry
var DefaultExecutionProfileRegistry = euclotypes.DefaultExecutionProfileRegistry
