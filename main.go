package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

type Output struct {
	RootModule    ModuleDetail   `json:"root_module"`
	LocalModules  []ModuleDetail `json:"local_modules"`
	RemoteModules []RemoteModule `json:"remote_modules"`
}

type ModuleDetail struct {
	Name         string   `json:"name,omitempty"`
	Source       string   `json:"source,omitempty"`
	ResolvedPath string   `json:"resolved_path"`
	Files        []string `json:"files"`
}

type RemoteModule struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Version    string `json:"version,omitempty"`
	CalledFrom string `json:"called_from"`
}

const (
	exitAffected    = 0
	exitNotAffected = 1
	exitError       = 2
)

func main() {
	filesOnly := flag.Bool("files-only", false, "output only file paths, one per line")
	filterStdin := flag.Bool("filter-stdin", false, "filter output to only files matching stdin (use with --files-only)")
	affected := flag.Bool("affected", false, "check if module is affected by changed files from stdin (exit 0=affected, 1=not affected)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <directory>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s /path/to/terraform\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --files-only /path/to/terraform\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  git diff --name-only | %s --files-only --filter-stdin /path/to/terraform\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  git diff --name-only | %s --affected /path/to/terraform && terraform plan\n", os.Args[0])
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(exitError)
	}

	dir := flag.Arg(0)

	output, err := Analyze(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitError)
	}

	if *affected {
		changedFiles, err := readStdin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(exitError)
		}
		if IsAffected(changedFiles, output) {
			os.Exit(exitAffected)
		} else {
			os.Exit(exitNotAffected)
		}
	}

	if *filesOnly {
		files := CollectAllFiles(output)

		if *filterStdin {
			changedFiles, err := readStdin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(exitError)
			}
			files = FilterRelatedFiles(files, changedFiles, output)
		}

		for _, f := range files {
			fmt.Println(f)
		}
	} else {
		jsonOutput, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(jsonOutput))
	}
}

func readStdin() ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func IsAffected(changedFiles []string, output *Output) bool {
	cwd, _ := os.Getwd()

	for _, f := range changedFiles {
		absPath := f
		if !filepath.IsAbs(f) {
			absPath = filepath.Join(cwd, f)
		}
		absPath, _ = filepath.Abs(absPath)

		if isInDirectory(absPath, output.RootModule.ResolvedPath) {
			return true
		}

		for _, localMod := range output.LocalModules {
			if isInDirectory(absPath, localMod.ResolvedPath) {
				return true
			}
		}
	}

	return false
}

func FilterRelatedFiles(allFiles []string, changedFiles []string, output *Output) []string {
	cwd, _ := os.Getwd()

	changedAbsPaths := make(map[string]bool)
	for _, f := range changedFiles {
		absPath := f
		if !filepath.IsAbs(f) {
			absPath = filepath.Join(cwd, f)
		}
		absPath, _ = filepath.Abs(absPath)
		changedAbsPaths[absPath] = true
	}

	affectedModulePaths := make(map[string]bool)

	for changedPath := range changedAbsPaths {
		if isInDirectory(changedPath, output.RootModule.ResolvedPath) {
			affectedModulePaths[output.RootModule.ResolvedPath] = true
		}

		for _, localMod := range output.LocalModules {
			if isInDirectory(changedPath, localMod.ResolvedPath) {
				affectedModulePaths[localMod.ResolvedPath] = true
			}
		}
	}

	var result []string
	seen := make(map[string]bool)

	if affectedModulePaths[output.RootModule.ResolvedPath] {
		for _, f := range output.RootModule.Files {
			if !seen[f] {
				seen[f] = true
				result = append(result, f)
			}
		}
	}

	for _, localMod := range output.LocalModules {
		if affectedModulePaths[localMod.ResolvedPath] {
			for _, f := range localMod.Files {
				if !seen[f] {
					seen[f] = true
					result = append(result, f)
				}
			}
		}
	}

	return result
}

func isInDirectory(filePath, dirPath string) bool {
	rel, err := filepath.Rel(dirPath, filePath)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func CollectAllFiles(output *Output) []string {
	seen := make(map[string]bool)
	var files []string

	for _, f := range output.RootModule.Files {
		if !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}

	for _, m := range output.LocalModules {
		for _, f := range m.Files {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}

	return files
}

func Analyze(dir string) (*Output, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	visited := make(map[string]bool)
	localModules := []ModuleDetail{}
	remoteModules := []RemoteModule{}

	rootFiles, err := listTerraformFiles(absDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list terraform files in root: %w", err)
	}

	rootModule := ModuleDetail{
		ResolvedPath: absDir,
		Files:        rootFiles,
	}

	err = analyzeRecursive(absDir, "", visited, &localModules, &remoteModules)
	if err != nil {
		return nil, err
	}

	return &Output{
		RootModule:    rootModule,
		LocalModules:  localModules,
		RemoteModules: remoteModules,
	}, nil
}

func analyzeRecursive(
	dir string,
	calledFrom string,
	visited map[string]bool,
	localModules *[]ModuleDetail,
	remoteModules *[]RemoteModule,
) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	if visited[absDir] {
		return nil
	}
	visited[absDir] = true

	module, diags := tfconfig.LoadModule(absDir)
	if diags.HasErrors() {
		return fmt.Errorf("failed to load module %s: %s", absDir, diags.Error())
	}

	for name, call := range module.ModuleCalls {
		if isLocalPath(call.Source) {
			resolvedPath := filepath.Join(absDir, call.Source)
			resolvedPath, _ = filepath.Abs(resolvedPath)

			files, err := listTerraformFiles(resolvedPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", resolvedPath, err)
				continue
			}

			*localModules = append(*localModules, ModuleDetail{
				Name:         name,
				Source:       call.Source,
				ResolvedPath: resolvedPath,
				Files:        files,
			})

			err = analyzeRecursive(resolvedPath, name, visited, localModules, remoteModules)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to analyze %s: %v\n", resolvedPath, err)
			}
		} else {
			caller := calledFrom
			if caller == "" {
				caller = "(root)"
			}
			*remoteModules = append(*remoteModules, RemoteModule{
				Name:       name,
				Source:     call.Source,
				Version:    call.Version,
				CalledFrom: caller,
			})
		}
	}

	return nil
}

func listTerraformFiles(dir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tf.json") {
			files = append(files, filepath.Join(dir, name))
		}
	}

	return files, nil
}

func isLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../")
}
