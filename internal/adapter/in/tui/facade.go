// Package tui exposes the Protonman fullscreen terminal adapter.
package tui

import runtimeui "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime"

type BubbleTeaUI = runtimeui.BubbleTeaUI
type BubbleTeaOption = runtimeui.BubbleTeaOption
type TodoHandlerFactory = runtimeui.TodoHandlerFactory

var NewBubbleTea = runtimeui.NewBubbleTea
var WithApplicationServices = runtimeui.WithApplicationServices
var WithBubbleTeaRunner = runtimeui.WithBubbleTeaRunner
var WithModelConfig = runtimeui.WithModelConfig
var WithSkills = runtimeui.WithSkills
var WithWorkDir = runtimeui.WithWorkDir
var WithInitialMessages = runtimeui.WithInitialMessages
var WithActiveGoal = runtimeui.WithActiveGoal
var WithLowConcurrencyMode = runtimeui.WithLowConcurrencyMode
var WithSessionID = runtimeui.WithSessionID
var WithSessions = runtimeui.WithSessions
var WithTodoHandlerFactory = runtimeui.WithTodoHandlerFactory
var WithAgentConfig = runtimeui.WithAgentConfig
var WithRuntimeConfig = runtimeui.WithRuntimeConfig
var WithProjectContext = runtimeui.WithProjectContext
var WithCoordinator = runtimeui.WithCoordinator
