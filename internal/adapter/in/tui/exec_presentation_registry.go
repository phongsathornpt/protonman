package tui

import "strings"

type execCommandProfile struct {
	Family    execFamily
	Matches   func(string) bool
	Action    func([]string) string
	Title     func(string, []string, string) string
	Summarize func(*execPresentation, string)
}

func exactExecNames(names ...string) func(string) bool {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return func(name string) bool { _, ok := set[name]; return ok }
}

var execCommandProfiles = []execCommandProfile{
	{execFamilyGit, exactExecNames("git"), gitAction, func(_ string, args []string, _ string) string { return execTitle("Git", strings.Join(args, " ")) }, summarizeGitExec},
	{execFamilyGo, exactExecNames("go"), firstArg, func(_ string, args []string, _ string) string { return execTitle("Go", strings.Join(args, " ")) }, summarizeGoExec},
	{execFamilyBun, exactExecNames("bun"), packageRunnerAction, func(_ string, args []string, _ string) string { return execTitle("Bun", strings.Join(args, " ")) }, summarizeBunExec},
	{execFamilyNode, exactExecNames("node"), nodeAction, func(_ string, args []string, action string) string { return nodeExecTitle(args, action) }, summarizeNodeExec},
	{execFamilyNode, exactExecNames("npm", "pnpm", "yarn"), packageRunnerAction, packageManagerExecTitle, summarizeNodeExec},
	{execFamilyPython, isPythonExecutable, pythonAction, func(_ string, args []string, action string) string { return pythonExecTitle(args, action) }, summarizePythonExec},
	{execFamilyRust, exactExecNames("cargo", "rustc", "rustfmt"), rustAction, rustExecTitle, summarizeRustExec},
	{execFamilyVite, exactExecNames("vite"), firstArg, func(_ string, _ []string, action string) string { return execTitle("Vite", action) }, summarizeViteExec},
	{execFamilyNext, exactExecNames("next"), firstArg, func(_ string, _ []string, action string) string { return execTitle("Next", action) }, summarizeNextExec},
}

func packageManagerExecTitle(name string, args []string, _ string) string {
	label := strings.ToUpper(name[:1]) + name[1:]
	return execTitle(label, strings.Join(args, " "))
}

func execProfileForCommand(name string) *execCommandProfile {
	for i := range execCommandProfiles {
		if execCommandProfiles[i].Matches(name) {
			return &execCommandProfiles[i]
		}
	}
	return nil
}

func execProfileForFamily(family execFamily) *execCommandProfile {
	for i := range execCommandProfiles {
		if execCommandProfiles[i].Family == family {
			return &execCommandProfiles[i]
		}
	}
	return nil
}
