package gha

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/sourceplane/tinx/internal/state"
)

const runtimeStateFile = "gha-runtime-state.json"

var expressionPattern = regexp.MustCompile(`\$\{\{\s*([^}]+?)\s*\}\}`)

type ExecuteOptions struct {
	Home        string
	Metadata    state.ProviderMetadata
	Capability  string
	Args        []string
	WorkingDir  string
	TinxVersion string
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       *os.File
}

type runtimeState struct {
	Env     map[string]string `json:"env,omitempty"`
	Path    []string          `json:"path,omitempty"`
	Outputs map[string]string `json:"outputs,omitempty"`
}

type expressionContext struct {
	inputs      map[string]string
	env         map[string]string
	stepOutputs map[string]map[string]string
	github      map[string]string
	runner      map[string]string
}

func Execute(opts ExecuteOptions) error {
	if opts.Capability != DefaultCapability {
		return fmt.Errorf("GitHub Action provider %s/%s does not expose capability %q", opts.Metadata.Namespace, opts.Metadata.Name, opts.Capability)
	}
	providerHome := state.VersionRoot(opts.Home, opts.Metadata.Namespace, opts.Metadata.Name, opts.Metadata.Version)
	sourcePath := opts.Metadata.Source.SourcePath
	if sourcePath == "" {
		sourcePath = filepath.Join(providerHome, "source")
	}
	actionPath := sourcePath
	if subpath := strings.TrimSpace(opts.Metadata.Source.Subpath); subpath != "" {
		actionPath = filepath.Join(sourcePath, filepath.FromSlash(subpath))
	}
	action, _, err := LoadAction(actionPath)
	if err != nil {
		return err
	}
	runtimeKind, err := ProviderRuntime(action.Runs.Using)
	if err != nil {
		return err
	}
	inputs, err := parseInputs(opts.Args)
	if err != nil {
		return err
	}
	resolvedInputs, err := resolveInputs(action, mergeInputs(opts.Metadata.Source.Inputs, inputs))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(providerHome, 0o755); err != nil {
		return fmt.Errorf("create provider runtime home: %w", err)
	}
	runnerHome := filepath.Join(providerHome, "home")
	runnerTemp := filepath.Join(providerHome, "tmp")
	runnerToolCache := filepath.Join(providerHome, "tool-cache")
	stepRoot := filepath.Join(providerHome, ".gha")
	for _, dir := range []string{runnerHome, runnerTemp, runnerToolCache, stepRoot, filepath.Join(providerHome, "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prepare GitHub Action runtime: %w", err)
		}
	}

	persisted, err := loadRuntimeState(providerHome)
	if err != nil {
		return err
	}
	currentEnv := envMapFromList(os.Environ())
	tinxPathEntries := []string{filepath.Join(providerHome, "bin")}
	if tinxExecutable, err := currentTinxExecutable(); err == nil {
		if err := ensureTinxShim(filepath.Join(providerHome, "bin"), tinxExecutable); err != nil {
			return err
		}
		tinxPathEntries = append(tinxPathEntries, filepath.Dir(tinxExecutable))
		currentEnv["TINX_BIN_DIR"] = filepath.Join(providerHome, "bin")
	}
	for key, value := range persisted.Env {
		currentEnv[key] = value
	}
	currentEnv["HOME"] = runnerHome
	currentEnv["RUNNER_TEMP"] = runnerTemp
	currentEnv["RUNNER_TOOL_CACHE"] = runnerToolCache
	currentEnv["AGENT_TOOLSDIRECTORY"] = runnerToolCache
	currentEnv["RUNNER_OS"] = githubRunnerOS(goruntime.GOOS)
	currentEnv["RUNNER_ARCH"] = githubRunnerArch(goruntime.GOARCH)
	currentEnv["GITHUB_WORKSPACE"] = opts.WorkingDir
	currentEnv["GITHUB_ACTION_PATH"] = actionPath
	currentEnv["GITHUB_ACTION_REPOSITORY"] = opts.Metadata.Source.Repo
	currentEnv["GITHUB_ACTION_REF"] = opts.Metadata.Version
	currentEnv["GITHUB_ACTIONS"] = "true"
	currentEnv["TINX_PROVIDER_HOME"] = providerHome
	currentEnv["TINX_VERSION"] = opts.TinxVersion
	currentEnv["TINX_CONTEXT"] = "{}"
	for name, value := range resolvedInputs {
		currentEnv[inputEnvName(name)] = value
	}
	pathEntries := prependPaths(append(tinxPathEntries, persisted.Path...), splitPathList(currentEnv["PATH"]))
	currentEnv["PATH"] = joinPathList(pathEntries)

	githubValues := map[string]string{
		"action_path":       actionPath,
		"workspace":         opts.WorkingDir,
		"action_repository": opts.Metadata.Source.Repo,
		"action_ref":        opts.Metadata.Version,
	}
	runnerValues := map[string]string{
		"temp": runnerTemp,
	}
	var outputs map[string]string
	switch runtimeKind {
	case RuntimeComposite:
		outputs, pathEntries, err = executeCompositeAction(action, opts, stepRoot, resolvedInputs, currentEnv, &persisted, pathEntries, githubValues, runnerValues)
	case RuntimeNode:
		outputs, pathEntries, err = executeNodeAction(action, opts, actionPath, stepRoot, resolvedInputs, currentEnv, &persisted, pathEntries, githubValues, runnerValues)
	default:
		return fmt.Errorf("GitHub Action runtime %q is not supported yet", action.Runs.Using)
	}
	if err != nil {
		return err
	}
	persisted.Outputs = outputs
	if err := saveRuntimeState(providerHome, persisted); err != nil {
		return err
	}
	return nil
}

func mergeInputs(configured, provided map[string]string) map[string]string {
	if len(configured) == 0 && len(provided) == 0 {
		return nil
	}
	merged := make(map[string]string, len(configured)+len(provided))
	for name, value := range configured {
		merged[name] = value
	}
	for name, value := range provided {
		merged[name] = value
	}
	return merged
}

func PassthroughEnvironment(home string, meta state.ProviderMetadata, tinxVersion string) (map[string]string, error) {
	providerHome := state.VersionRoot(home, meta.Namespace, meta.Name, meta.Version)
	persisted, err := loadRuntimeState(providerHome)
	if err != nil {
		return nil, err
	}
	binDir := filepath.Join(providerHome, "bin")
	tinxPathEntries := []string{binDir}
	env := cloneMap(persisted.Env)
	if tinxExecutable, err := currentTinxExecutable(); err == nil {
		if err := ensureTinxShim(binDir, tinxExecutable); err != nil {
			return nil, err
		}
		tinxPathEntries = append(tinxPathEntries, filepath.Dir(tinxExecutable))
		env["TINX_BIN_DIR"] = binDir
	}
	env["HOME"] = filepath.Join(providerHome, "home")
	env["RUNNER_TEMP"] = filepath.Join(providerHome, "tmp")
	env["RUNNER_TOOL_CACHE"] = filepath.Join(providerHome, "tool-cache")
	env["AGENT_TOOLSDIRECTORY"] = env["RUNNER_TOOL_CACHE"]
	env["PATH"] = joinPathList(prependPaths(append(tinxPathEntries, persisted.Path...), splitPathList(os.Getenv("PATH"))))
	env["TINX_PROVIDER_HOME"] = providerHome
	env["TINX_VERSION"] = tinxVersion
	env["TINX_CONTEXT"] = "{}"
	return env, nil
}

func executeCompositeAction(action Action, opts ExecuteOptions, stepRoot string, resolvedInputs map[string]string, currentEnv map[string]string, persisted *runtimeState, pathEntries []string, githubValues, runnerValues map[string]string) (map[string]string, []string, error) {
	stepOutputs := make(map[string]map[string]string)
	for index, step := range action.Runs.Steps {
		if step.Uses != "" {
			return nil, pathEntries, fmt.Errorf("GitHub Action %s uses nested action %q; nested uses steps are not supported yet", opts.Metadata.Source.Repo, step.Uses)
		}
		if strings.TrimSpace(step.Run) == "" {
			continue
		}
		ctx := expressionContext{
			inputs:      resolvedInputs,
			env:         cloneMap(currentEnv),
			stepOutputs: stepOutputs,
			github:      githubValues,
			runner:      runnerValues,
		}
		stepName := describeStep(index, step)
		stepDir := opts.WorkingDir
		if step.WorkingDirectory != "" {
			renderedDir, err := renderExpressionTemplate(step.WorkingDirectory, ctx)
			if err != nil {
				return nil, pathEntries, fmt.Errorf("render working-directory for %s: %w", stepName, err)
			}
			if filepath.IsAbs(renderedDir) {
				stepDir = renderedDir
			} else {
				stepDir = filepath.Join(opts.WorkingDir, renderedDir)
			}
		}
		if _, err := os.Stat(stepDir); err != nil {
			return nil, pathEntries, fmt.Errorf("prepare working-directory for %s: %w", stepName, err)
		}

		stepEnv := cloneMap(currentEnv)
		for _, key := range sortedKeys(step.Env) {
			value, err := renderExpressionTemplate(step.Env[key], expressionContext{
				inputs:      resolvedInputs,
				env:         stepEnv,
				stepOutputs: stepOutputs,
				github:      githubValues,
				runner:      runnerValues,
			})
			if err != nil {
				return nil, pathEntries, fmt.Errorf("render env %s for %s: %w", key, stepName, err)
			}
			stepEnv[key] = value
		}

		files, err := prepareCommandFiles(stepRoot, fmt.Sprintf("step-%03d", index+1))
		if err != nil {
			return nil, pathEntries, err
		}
		stepEnv["GITHUB_ENV"] = files.EnvPath
		stepEnv["GITHUB_PATH"] = files.PathPath
		stepEnv["GITHUB_OUTPUT"] = files.OutputPath
		stepEnv["GITHUB_STATE"] = files.StatePath

		renderedRun, err := renderExpressionTemplate(step.Run, expressionContext{
			inputs:      resolvedInputs,
			env:         stepEnv,
			stepOutputs: stepOutputs,
			github:      githubValues,
			runner:      runnerValues,
		})
		if err != nil {
			return nil, pathEntries, fmt.Errorf("render step %s: %w", stepName, err)
		}
		command, commandArgs, err := shellCommand(step.Shell, renderedRun)
		if err != nil {
			return nil, pathEntries, fmt.Errorf("prepare shell for %s: %w", stepName, err)
		}
		cmd := osexec.Command(command, commandArgs...)
		cmd.Dir = stepDir
		cmd.Env = envListFromMap(stepEnv)
		cmd.Stdout = opts.Stdout
		cmd.Stderr = opts.Stderr
		cmd.Stdin = opts.Stdin
		if err := cmd.Run(); err != nil {
			return nil, pathEntries, fmt.Errorf("execute %s: %w", stepName, err)
		}

		stepOutputValues, nextPathEntries, err := applyCommandFiles(files, stepName, stepDir, currentEnv, persisted, pathEntries)
		if err != nil {
			return nil, pathEntries, err
		}
		pathEntries = nextPathEntries
		if step.ID != "" {
			stepOutputs[step.ID] = stepOutputValues
		}
	}

	outputs := make(map[string]string, len(action.Outputs))
	for name, output := range action.Outputs {
		value, err := renderExpressionTemplate(output.Value, expressionContext{
			inputs:      resolvedInputs,
			env:         currentEnv,
			stepOutputs: stepOutputs,
			github:      githubValues,
			runner:      runnerValues,
		})
		if err != nil {
			return nil, pathEntries, fmt.Errorf("render output %s: %w", name, err)
		}
		outputs[name] = value
	}
	return outputs, pathEntries, nil
}

func executeNodeAction(action Action, opts ExecuteOptions, actionPath, stepRoot string, resolvedInputs map[string]string, currentEnv map[string]string, persisted *runtimeState, pathEntries []string, githubValues, runnerValues map[string]string) (map[string]string, []string, error) {
	nodeBinary, err := osexec.LookPath("node")
	if err != nil {
		return nil, pathEntries, fmt.Errorf("GitHub Action %s requires a node runtime (%s), but node was not found on PATH", opts.Metadata.Source.Repo, action.Runs.Using)
	}
	mainEntrypoint, err := MainEntrypoint(action)
	if err != nil {
		return nil, pathEntries, err
	}
	files, err := prepareCommandFiles(stepRoot, "node")
	if err != nil {
		return nil, pathEntries, err
	}
	nodeEnv := cloneMap(currentEnv)
	nodeEnv["GITHUB_ENV"] = files.EnvPath
	nodeEnv["GITHUB_PATH"] = files.PathPath
	nodeEnv["GITHUB_OUTPUT"] = files.OutputPath
	nodeEnv["GITHUB_STATE"] = files.StatePath

	cmd := osexec.Command(nodeBinary, filepath.Join(actionPath, filepath.FromSlash(mainEntrypoint)))
	cmd.Dir = opts.WorkingDir
	cmd.Env = envListFromMap(nodeEnv)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	if err := cmd.Run(); err != nil {
		return nil, pathEntries, fmt.Errorf("execute node action: %w", err)
	}

	rawOutputs, nextPathEntries, err := applyCommandFiles(files, "node action", opts.WorkingDir, currentEnv, persisted, pathEntries)
	if err != nil {
		return nil, pathEntries, err
	}
	outputs := cloneMap(rawOutputs)
	for name, output := range action.Outputs {
		if strings.TrimSpace(output.Value) == "" {
			continue
		}
		value, err := renderExpressionTemplate(output.Value, expressionContext{
			inputs: resolvedInputs,
			env:    currentEnv,
			github: githubValues,
			runner: runnerValues,
		})
		if err != nil {
			return nil, pathEntries, fmt.Errorf("render output %s: %w", name, err)
		}
		outputs[name] = value
	}
	return outputs, nextPathEntries, nil
}

type commandFiles struct {
	Dir        string
	EnvPath    string
	PathPath   string
	OutputPath string
	StatePath  string
}

func prepareCommandFiles(stepRoot, name string) (commandFiles, error) {
	directory := filepath.Join(stepRoot, name)
	if err := os.RemoveAll(directory); err != nil {
		return commandFiles{}, fmt.Errorf("reset GitHub Action command files: %w", err)
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return commandFiles{}, fmt.Errorf("create GitHub Action command files: %w", err)
	}
	files := commandFiles{
		Dir:        directory,
		EnvPath:    filepath.Join(directory, "env"),
		PathPath:   filepath.Join(directory, "path"),
		OutputPath: filepath.Join(directory, "output"),
		StatePath:  filepath.Join(directory, "state"),
	}
	for _, filePath := range []string{files.EnvPath, files.PathPath, files.OutputPath, files.StatePath} {
		if err := os.WriteFile(filePath, nil, 0o644); err != nil {
			return commandFiles{}, fmt.Errorf("initialize GitHub Action command file: %w", err)
		}
	}
	return files, nil
}

func applyCommandFiles(files commandFiles, description, workingDir string, currentEnv map[string]string, persisted *runtimeState, pathEntries []string) (map[string]string, []string, error) {
	newEnv, err := parseCommandFile(files.EnvPath)
	if err != nil {
		return nil, pathEntries, fmt.Errorf("parse GITHUB_ENV for %s: %w", description, err)
	}
	for key, value := range newEnv {
		persisted.Env[key] = value
		currentEnv[key] = value
	}
	newPaths, err := parsePathFile(files.PathPath, workingDir)
	if err != nil {
		return nil, pathEntries, fmt.Errorf("parse GITHUB_PATH for %s: %w", description, err)
	}
	persisted.Path = prependPaths(newPaths, persisted.Path)
	pathEntries = prependPaths(newPaths, pathEntries)
	currentEnv["PATH"] = joinPathList(pathEntries)
	outputs, err := parseCommandFile(files.OutputPath)
	if err != nil {
		return nil, pathEntries, fmt.Errorf("parse GITHUB_OUTPUT for %s: %w", description, err)
	}
	return outputs, pathEntries, nil
}

func githubRunnerOS(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	default:
		return "Linux"
	}
}

func githubRunnerArch(goarch string) string {
	switch goarch {
	case "386":
		return "X86"
	case "arm":
		return "ARM"
	case "arm64":
		return "ARM64"
	default:
		return "X64"
	}
}

func parseInputs(args []string) (map[string]string, error) {
	inputs := make(map[string]string)
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--input":
			if index+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --input")
			}
			name, value, err := splitInputArg(args[index+1])
			if err != nil {
				return nil, err
			}
			inputs[name] = value
			index++
		case strings.HasPrefix(arg, "--input="):
			name, value, err := splitInputArg(strings.TrimPrefix(arg, "--input="))
			if err != nil {
				return nil, err
			}
			inputs[name] = value
		default:
			return nil, fmt.Errorf("unsupported GitHub Action argument %q; use --input name=value", arg)
		}
	}
	return inputs, nil
}

func splitInputArg(value string) (string, string, error) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", fmt.Errorf("GitHub Action inputs must use name=value syntax, got %q", value)
	}
	return strings.TrimSpace(parts[0]), parts[1], nil
}

func resolveInputs(action Action, provided map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(action.Inputs))
	for name, spec := range action.Inputs {
		if value, ok := provided[name]; ok {
			resolved[name] = value
			continue
		}
		if spec.Default != nil {
			resolved[name] = scalarString(spec.Default)
			continue
		}
		if spec.Required {
			return nil, fmt.Errorf("missing required GitHub Action input %q", name)
		}
	}
	for name, value := range provided {
		if _, ok := action.Inputs[name]; !ok {
			return nil, fmt.Errorf("unknown GitHub Action input %q", name)
		}
		resolved[name] = value
	}
	return resolved, nil
}

func shellCommand(shell, script string) (string, []string, error) {
	switch strings.TrimSpace(shell) {
	case "", "sh":
		return "sh", []string{"-e", "-c", script}, nil
	case "bash":
		return "bash", []string{"--noprofile", "--norc", "-eo", "pipefail", "-c", script}, nil
	default:
		return "", nil, fmt.Errorf("unsupported shell %q", shell)
	}
}

func renderExpressionTemplate(template string, ctx expressionContext) (string, error) {
	matches := expressionPattern.FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		return template, nil
	}
	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(template[last:match[0]])
		expression := strings.TrimSpace(template[match[2]:match[3]])
		value, err := resolveExpression(expression, ctx)
		if err != nil {
			return "", err
		}
		builder.WriteString(value)
		last = match[1]
	}
	builder.WriteString(template[last:])
	return builder.String(), nil
}

func resolveExpression(expression string, ctx expressionContext) (string, error) {
	switch {
	case strings.HasPrefix(expression, "inputs."):
		return ctx.inputs[strings.TrimPrefix(expression, "inputs.")], nil
	case strings.HasPrefix(expression, "env."):
		return ctx.env[strings.TrimPrefix(expression, "env.")], nil
	case strings.HasPrefix(expression, "github."):
		return ctx.github[strings.TrimPrefix(expression, "github.")], nil
	case strings.HasPrefix(expression, "runner."):
		return ctx.runner[strings.TrimPrefix(expression, "runner.")], nil
	case strings.HasPrefix(expression, "steps."):
		parts := strings.Split(expression, ".")
		if len(parts) == 4 && parts[2] == "outputs" {
			if stepValues, ok := ctx.stepOutputs[parts[1]]; ok {
				return stepValues[parts[3]], nil
			}
			return "", nil
		}
	}
	return "", fmt.Errorf("unsupported GitHub Actions expression %q", expression)
}

func parseCommandFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if line == "" {
			continue
		}
		if marker := strings.Index(line, "<<"); marker > 0 {
			name := line[:marker]
			delimiter := line[marker+2:]
			index++
			start := index
			for index < len(lines) && lines[index] != delimiter {
				index++
			}
			if index >= len(lines) {
				return nil, fmt.Errorf("unterminated multiline command for %s", name)
			}
			values[name] = strings.Join(lines[start:index], "\n")
			continue
		}
		separator := strings.Index(line, "=")
		if separator <= 0 {
			return nil, fmt.Errorf("invalid command file entry %q", line)
		}
		values[line[:separator]] = line[separator+1:]
	}
	return values, nil
}

func parsePathFile(path, workingDir string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if filepath.IsAbs(line) {
			paths = append(paths, filepath.Clean(line))
			continue
		}
		paths = append(paths, filepath.Clean(filepath.Join(workingDir, line)))
	}
	return paths, nil
}

func loadRuntimeState(providerHome string) (runtimeState, error) {
	statePath := filepath.Join(providerHome, runtimeStateFile)
	var current runtimeState
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			current.Env = make(map[string]string)
			return current, nil
		}
		return current, fmt.Errorf("read GitHub Action runtime state: %w", err)
	}
	if err := json.Unmarshal(data, &current); err != nil {
		return current, fmt.Errorf("decode GitHub Action runtime state: %w", err)
	}
	if current.Env == nil {
		current.Env = make(map[string]string)
	}
	if current.Outputs == nil {
		current.Outputs = make(map[string]string)
	}
	return current, nil
}

func saveRuntimeState(providerHome string, state runtimeState) error {
	statePath := filepath.Join(providerHome, runtimeStateFile)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode GitHub Action runtime state: %w", err)
	}
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		return fmt.Errorf("write GitHub Action runtime state: %w", err)
	}
	return nil
}

func envMapFromList(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[parts[0]] = parts[1]
	}
	return result
}

func envListFromMap(values map[string]string) []string {
	keys := sortedKeys(values)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func sortedKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func inputEnvName(name string) string {
	var builder strings.Builder
	builder.WriteString("INPUT_")
	for _, r := range strings.ToUpper(name) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	return builder.String()
}

func describeStep(index int, step ActionStep) string {
	if step.Name != "" {
		return step.Name
	}
	if step.ID != "" {
		return step.ID
	}
	return fmt.Sprintf("step-%d", index+1)
}

func splitPathList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, string(os.PathListSeparator))
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		paths = append(paths, part)
	}
	return paths
}

func joinPathList(paths []string) string {
	return strings.Join(paths, string(os.PathListSeparator))
}

func prependPaths(additions, existing []string) []string {
	result := make([]string, 0, len(additions)+len(existing))
	seen := make(map[string]struct{}, len(additions)+len(existing))
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, value := range existing {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func currentTinxExecutable() (string, error) {
	if override := strings.TrimSpace(os.Getenv("TINX_GHA_TINX_PATH")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("stat TINX_GHA_TINX_PATH: %w", err)
		}
		return override, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve tinx executable: %w", err)
	}
	return executable, nil
}

func ensureTinxShim(binDir, executable string) error {
	if executable == "" {
		return nil
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create tinx shim directory: %w", err)
	}
	shimPath := filepath.Join(binDir, "tinx")
	content := fmt.Sprintf("#!/bin/sh\nexec %q \"$@\"\n", executable)
	if err := os.WriteFile(shimPath, []byte(content), 0o755); err != nil {
		return fmt.Errorf("write tinx shim: %w", err)
	}
	return nil
}
