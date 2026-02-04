package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyze(t *testing.T) {
	tempDir := t.TempDir()

	rootDir := filepath.Join(tempDir, "root")
	moduleDir := filepath.Join(tempDir, "modules", "vpc")
	nestedModuleDir := filepath.Join(tempDir, "modules", "vpc", "subnets")

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedModuleDir, 0755); err != nil {
		t.Fatal(err)
	}

	rootMain := `
module "vpc" {
  source = "../modules/vpc"
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"
}
`
	if err := os.WriteFile(filepath.Join(rootDir, "main.tf"), []byte(rootMain), 0644); err != nil {
		t.Fatal(err)
	}

	vpcMain := `
module "subnets" {
  source = "./subnets"
}

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(vpcMain), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	subnetsMain := `
resource "aws_subnet" "main" {
  vpc_id     = var.vpc_id
  cidr_block = "10.0.1.0/24"
}
`
	if err := os.WriteFile(filepath.Join(nestedModuleDir, "main.tf"), []byte(subnetsMain), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := Analyze(rootDir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if output.RootModule.ResolvedPath != rootDir {
		t.Errorf("expected root path %s, got %s", rootDir, output.RootModule.ResolvedPath)
	}

	if len(output.RootModule.Files) != 1 {
		t.Errorf("expected 1 root file, got %d", len(output.RootModule.Files))
	}

	if len(output.LocalModules) != 2 {
		t.Errorf("expected 2 local modules (vpc and subnets), got %d", len(output.LocalModules))
	}

	if len(output.RemoteModules) != 1 {
		t.Errorf("expected 1 remote module, got %d", len(output.RemoteModules))
	}

	if output.RemoteModules[0].Source != "terraform-aws-modules/eks/aws" {
		t.Errorf("expected eks module source, got %s", output.RemoteModules[0].Source)
	}

	if output.RemoteModules[0].Version != "~> 19.0" {
		t.Errorf("expected version ~> 19.0, got %s", output.RemoteModules[0].Version)
	}
}

func TestAnalyze_CircularDependency(t *testing.T) {
	tempDir := t.TempDir()

	moduleA := filepath.Join(tempDir, "module_a")
	moduleB := filepath.Join(tempDir, "module_b")

	if err := os.MkdirAll(moduleA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleB, 0755); err != nil {
		t.Fatal(err)
	}

	moduleAMain := `
module "b" {
  source = "../module_b"
}
`
	if err := os.WriteFile(filepath.Join(moduleA, "main.tf"), []byte(moduleAMain), 0644); err != nil {
		t.Fatal(err)
	}

	moduleBMain := `
module "a" {
  source = "../module_a"
}
`
	if err := os.WriteFile(filepath.Join(moduleB, "main.tf"), []byte(moduleBMain), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := Analyze(moduleA)
	if err != nil {
		t.Fatalf("Analyze should not fail on circular dependency: %v", err)
	}

	if len(output.LocalModules) != 2 {
		t.Errorf("expected 2 local modules, got %d", len(output.LocalModules))
	}
}

func TestAnalyze_EmptyDir(t *testing.T) {
	tempDir := t.TempDir()

	output, err := Analyze(tempDir)
	if err != nil {
		t.Fatalf("Analyze failed on empty dir: %v", err)
	}

	if len(output.RootModule.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(output.RootModule.Files))
	}

	if len(output.LocalModules) != 0 {
		t.Errorf("expected 0 local modules, got %d", len(output.LocalModules))
	}

	if len(output.RemoteModules) != 0 {
		t.Errorf("expected 0 remote modules, got %d", len(output.RemoteModules))
	}
}

func TestListTerraformFiles(t *testing.T) {
	tempDir := t.TempDir()

	testFiles := []string{"main.tf", "variables.tf", "outputs.tf.json", "readme.md", "script.sh"}
	for _, f := range testFiles {
		if err := os.WriteFile(filepath.Join(tempDir, f), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := listTerraformFiles(tempDir)
	if err != nil {
		t.Fatalf("listTerraformFiles failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 terraform files, got %d", len(files))
	}
}

func TestCollectAllFiles(t *testing.T) {
	tempDir := t.TempDir()

	rootDir := filepath.Join(tempDir, "root")
	moduleDir := filepath.Join(tempDir, "modules", "vpc")

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}

	rootMain := `
module "vpc" {
  source = "../modules/vpc"
}
`
	if err := os.WriteFile(filepath.Join(rootDir, "main.tf"), []byte(rootMain), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "outputs.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := Analyze(rootDir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	files := CollectAllFiles(output)

	if len(files) != 4 {
		t.Errorf("expected 4 files, got %d: %v", len(files), files)
	}

	seen := make(map[string]bool)
	for _, f := range files {
		if seen[f] {
			t.Errorf("duplicate file: %s", f)
		}
		seen[f] = true
	}
}

func TestFilterRelatedFiles(t *testing.T) {
	tempDir := t.TempDir()

	rootDir := filepath.Join(tempDir, "root")
	moduleDir := filepath.Join(tempDir, "modules", "vpc")
	otherModuleDir := filepath.Join(tempDir, "modules", "iam")

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherModuleDir, 0755); err != nil {
		t.Fatal(err)
	}

	rootMain := `
module "vpc" {
  source = "../modules/vpc"
}
module "iam" {
  source = "../modules/iam"
}
`
	if err := os.WriteFile(filepath.Join(rootDir, "main.tf"), []byte(rootMain), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "variables.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherModuleDir, "main.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := Analyze(rootDir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	allFiles := CollectAllFiles(output)

	t.Run("filter to vpc module only", func(t *testing.T) {
		changedFiles := []string{filepath.Join(moduleDir, "main.tf")}
		filtered := FilterRelatedFiles(allFiles, changedFiles, output)

		if len(filtered) != 2 {
			t.Errorf("expected 2 files (vpc module files), got %d: %v", len(filtered), filtered)
		}

		for _, f := range filtered {
			if !isInDirectory(f, moduleDir) {
				t.Errorf("unexpected file %s not in vpc module", f)
			}
		}
	})

	t.Run("filter to root module only", func(t *testing.T) {
		changedFiles := []string{filepath.Join(rootDir, "main.tf")}
		filtered := FilterRelatedFiles(allFiles, changedFiles, output)

		if len(filtered) != 1 {
			t.Errorf("expected 1 file (root main.tf), got %d: %v", len(filtered), filtered)
		}
	})

	t.Run("no matching files", func(t *testing.T) {
		changedFiles := []string{"/some/other/path/file.tf"}
		filtered := FilterRelatedFiles(allFiles, changedFiles, output)

		if len(filtered) != 0 {
			t.Errorf("expected 0 files, got %d: %v", len(filtered), filtered)
		}
	})
}

func TestIsInDirectory(t *testing.T) {
	tests := []struct {
		filePath string
		dirPath  string
		expected bool
	}{
		{"/a/b/c/file.tf", "/a/b/c", true},
		{"/a/b/c/d/file.tf", "/a/b/c", true},
		{"/a/b/file.tf", "/a/b/c", false},
		{"/a/b/c", "/a/b/c", true},
		{"/other/path/file.tf", "/a/b/c", false},
	}

	for _, tt := range tests {
		t.Run(tt.filePath+"_in_"+tt.dirPath, func(t *testing.T) {
			result := isInDirectory(tt.filePath, tt.dirPath)
			if result != tt.expected {
				t.Errorf("isInDirectory(%q, %q) = %v, expected %v", tt.filePath, tt.dirPath, result, tt.expected)
			}
		})
	}
}

func TestIsAffected(t *testing.T) {
	tempDir := t.TempDir()

	rootDir := filepath.Join(tempDir, "root")
	moduleDir := filepath.Join(tempDir, "modules", "vpc")

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatal(err)
	}

	rootMain := `
module "vpc" {
  source = "../modules/vpc"
}
`
	if err := os.WriteFile(filepath.Join(rootDir, "main.tf"), []byte(rootMain), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	output, err := Analyze(rootDir)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	t.Run("affected by root module change", func(t *testing.T) {
		changedFiles := []string{filepath.Join(rootDir, "main.tf")}
		if !IsAffected(changedFiles, output) {
			t.Error("expected affected=true for root module change")
		}
	})

	t.Run("affected by local module change", func(t *testing.T) {
		changedFiles := []string{filepath.Join(moduleDir, "main.tf")}
		if !IsAffected(changedFiles, output) {
			t.Error("expected affected=true for local module change")
		}
	})

	t.Run("not affected by unrelated change", func(t *testing.T) {
		changedFiles := []string{"/some/other/path/file.tf"}
		if IsAffected(changedFiles, output) {
			t.Error("expected affected=false for unrelated change")
		}
	})

	t.Run("not affected by empty change list", func(t *testing.T) {
		changedFiles := []string{}
		if IsAffected(changedFiles, output) {
			t.Error("expected affected=false for empty change list")
		}
	})
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		source   string
		expected bool
	}{
		{"./modules/vpc", true},
		{"../modules/vpc", true},
		{"../../shared/modules", true},
		{"terraform-aws-modules/eks/aws", false},
		{"git::https://github.com/org/repo.git", false},
		{"s3::https://bucket.s3.amazonaws.com/module.zip", false},
		{"registry.terraform.io/hashicorp/consul/aws", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := isLocalPath(tt.source)
			if result != tt.expected {
				t.Errorf("isLocalPath(%q) = %v, expected %v", tt.source, result, tt.expected)
			}
		})
	}
}
