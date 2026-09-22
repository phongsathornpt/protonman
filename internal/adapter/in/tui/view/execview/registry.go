package execview

import "strings"

type execCommandProfile struct {
	Family  Family
	Matches func(string) bool
	Action  func([]string) string
	Title   func(string, []string, string) string
}

// Rows match INDIVIDUAL commands for action/title only. Output summarization
// is family-level and lives in familySummarizers, so rows that share a family
// (node vs npm/pnpm/yarn) can never drift into competing summarizers.
func exactExecNames(names ...string) func(string) bool {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return func(name string) bool { _, ok := set[name]; return ok }
}

var execCommandProfiles = []execCommandProfile{
	{FamilyGit, exactExecNames("git"), gitAction, func(_ string, args []string, _ string) string { return execTitle("Git", strings.Join(args, " ")) }},
	{FamilyGo, exactExecNames("go"), firstArg, func(_ string, args []string, _ string) string { return execTitle("Go", strings.Join(args, " ")) }},
	{FamilyBun, exactExecNames("bun"), packageRunnerAction, func(_ string, args []string, _ string) string { return execTitle("Bun", strings.Join(args, " ")) }},
	{FamilyNode, exactExecNames("node"), nodeAction, func(_ string, args []string, action string) string { return nodeExecTitle(args, action) }},
	{FamilyNode, exactExecNames("npm", "pnpm", "yarn"), packageRunnerAction, packageManagerExecTitle},
	{FamilyPython, isPythonExecutable, pythonAction, func(_ string, args []string, action string) string { return pythonExecTitle(args, action) }},
	{FamilyRust, exactExecNames("cargo", "rustc", "rustfmt"), rustAction, rustExecTitle},
	{FamilyMake, exactExecNames("make", "gmake"), makeAction, makeExecTitle},
	{FamilyDocker, exactExecNames("docker", "docker-compose"), dockerAction, dockerExecTitle},
	{FamilyJVM, exactExecNames("java", "javac", "gradle", "gradlew", "mvn", "mvnw"), jvmAction, jvmExecTitle},
	{FamilyPHP, isPHPExecutable, phpAction, phpExecTitle},
	{FamilyRuby, isRubyExecutable, rubyAction, rubyExecTitle},
	{FamilyDotnet, exactExecNames("dotnet"), firstArg, dotnetExecTitle},
	{FamilyTerraform, exactExecNames("terraform", "tofu"), firstArg, terraformExecTitle},
	{FamilyKubectl, exactExecNames("kubectl"), kubectlAction, kubectlExecTitle},
	{FamilyVite, exactExecNames("vite"), firstArg, func(_ string, _ []string, action string) string { return execTitle("Vite", action) }},
	{FamilyNext, exactExecNames("next"), firstArg, func(_ string, _ []string, action string) string { return execTitle("Next", action) }},
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

// familySummarizers assigns exactly one summarizer per family. As a Go map
// literal, duplicate Family keys fail to compile, so a family can never grow
// competing per-row summarizers (the old first-match footgun).
var familySummarizers = map[Family]func(*Presentation, string){
	FamilyGit:       summarizeGitExec,
	FamilyGo:        summarizeGoExec,
	FamilyBun:       summarizeBunExec,
	FamilyNode:      summarizeNodeExec,
	FamilyPython:    summarizePythonExec,
	FamilyRust:      summarizeRustExec,
	FamilyMake:      summarizeMakeExec,
	FamilyDocker:    summarizeDockerExec,
	FamilyJVM:       summarizeJVMExec,
	FamilyPHP:       summarizePHPExec,
	FamilyRuby:      summarizeRubyExec,
	FamilyDotnet:    summarizeDotnetExec,
	FamilyTerraform: summarizeTerraformExec,
	FamilyKubectl:   summarizeKubectlExec,
	FamilyVite:      summarizeViteExec,
	FamilyNext:      summarizeNextExec,
}
