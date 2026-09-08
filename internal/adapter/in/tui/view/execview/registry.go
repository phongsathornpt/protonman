package execview

import "strings"

type execCommandProfile struct {
	Family    Family
	Matches   func(string) bool
	Action    func([]string) string
	Title     func(string, []string, string) string
	Summarize func(*Presentation, string)
}

func exactExecNames(names ...string) func(string) bool {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return func(name string) bool { _, ok := set[name]; return ok }
}

var execCommandProfiles = []execCommandProfile{
	{FamilyGit, exactExecNames("git"), gitAction, func(_ string, args []string, _ string) string { return execTitle("Git", strings.Join(args, " ")) }, summarizeGitExec},
	{FamilyGo, exactExecNames("go"), firstArg, func(_ string, args []string, _ string) string { return execTitle("Go", strings.Join(args, " ")) }, summarizeGoExec},
	{FamilyBun, exactExecNames("bun"), packageRunnerAction, func(_ string, args []string, _ string) string { return execTitle("Bun", strings.Join(args, " ")) }, summarizeBunExec},
	{FamilyNode, exactExecNames("node"), nodeAction, func(_ string, args []string, action string) string { return nodeExecTitle(args, action) }, summarizeNodeExec},
	{FamilyNode, exactExecNames("npm", "pnpm", "yarn"), packageRunnerAction, packageManagerExecTitle, summarizeNodeExec},
	{FamilyPython, isPythonExecutable, pythonAction, func(_ string, args []string, action string) string { return pythonExecTitle(args, action) }, summarizePythonExec},
	{FamilyRust, exactExecNames("cargo", "rustc", "rustfmt"), rustAction, rustExecTitle, summarizeRustExec},
	{FamilyMake, exactExecNames("make", "gmake"), makeAction, makeExecTitle, summarizeMakeExec},
	{FamilyDocker, exactExecNames("docker", "docker-compose"), dockerAction, dockerExecTitle, summarizeDockerExec},
	{FamilyJVM, exactExecNames("java", "javac", "gradle", "gradlew", "mvn", "mvnw"), jvmAction, jvmExecTitle, summarizeJVMExec},
	{FamilyPHP, isPHPExecutable, phpAction, phpExecTitle, summarizePHPExec},
	{FamilyRuby, isRubyExecutable, rubyAction, rubyExecTitle, summarizeRubyExec},
	{FamilyDotnet, exactExecNames("dotnet"), firstArg, dotnetExecTitle, summarizeDotnetExec},
	{FamilyTerraform, exactExecNames("terraform", "tofu"), firstArg, terraformExecTitle, summarizeTerraformExec},
	{FamilyKubectl, exactExecNames("kubectl"), kubectlAction, kubectlExecTitle, summarizeKubectlExec},
	{FamilyVite, exactExecNames("vite"), firstArg, func(_ string, _ []string, action string) string { return execTitle("Vite", action) }, summarizeViteExec},
	{FamilyNext, exactExecNames("next"), firstArg, func(_ string, _ []string, action string) string { return execTitle("Next", action) }, summarizeNextExec},
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

func execProfileForFamily(family Family) *execCommandProfile {
	for i := range execCommandProfiles {
		if execCommandProfiles[i].Family == family {
			return &execCommandProfiles[i]
		}
	}
	return nil
}
