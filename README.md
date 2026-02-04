# terraform-module-resolve

A CLI tool to analyze Terraform modules and list all related files, including files from local module dependencies.

## Features

- Recursively analyze Terraform modules and their local dependencies
- List all `.tf` and `.tf.json` files in a module and its dependencies
- Detect both local and remote module references
- Filter output based on changed files (useful for CI/CD pipelines)
- Check if a module is affected by file changes

## Installation

```bash
go install github.com/mkusaka/terraform-module-resolve@latest
```

Or build from source:

```bash
git clone https://github.com/mkusaka/terraform-module-resolve.git
cd terraform-module-resolve
go build -o terraform-module-resolve
```

## Usage

### Basic Usage

Analyze a Terraform module and output JSON:

```bash
terraform-module-resolve /path/to/terraform/module
```

Output:

```json
{
  "root_module": {
    "resolved_path": "/path/to/terraform/module",
    "files": [
      "/path/to/terraform/module/main.tf",
      "/path/to/terraform/module/variables.tf"
    ]
  },
  "local_modules": [
    {
      "name": "vpc",
      "source": "../modules/vpc",
      "resolved_path": "/path/to/modules/vpc",
      "files": [
        "/path/to/modules/vpc/main.tf",
        "/path/to/modules/vpc/outputs.tf"
      ]
    }
  ],
  "remote_modules": [
    {
      "name": "eks",
      "source": "terraform-aws-modules/eks/aws",
      "version": "~> 19.0",
      "called_from": "(root)"
    }
  ]
}
```

### List Files Only

Output only file paths, one per line:

```bash
terraform-module-resolve --files-only /path/to/terraform/module
```

### Filter by Changed Files

Filter output to only files in modules affected by changes from stdin:

```bash
git diff --name-only | terraform-module-resolve --files-only --filter-stdin /path/to/terraform/module
```

### Check if Module is Affected

Check if a module is affected by changed files. Useful for conditional CI/CD execution:

```bash
git diff --name-only | terraform-module-resolve --affected /path/to/terraform/module
```

Exit codes:
- `0`: Module is affected by the changes
- `1`: Module is not affected
- `2`: Error occurred

Example in CI:

```bash
if git diff --name-only origin/main | terraform-module-resolve --affected ./terraform/dev; then
  terraform plan
else
  echo "No changes affecting this module"
fi
```

## Options

| Flag | Description |
|------|-------------|
| `--files-only` | Output only file paths, one per line |
| `--filter-stdin` | Filter output to only files in modules matching stdin input (use with `--files-only`) |
| `--affected` | Check if module is affected by changed files from stdin (exit 0=affected, 1=not affected) |

## Use Cases

### CI/CD: Run Terraform Only for Affected Modules

```yaml
- name: Check if module affected
  id: check
  run: |
    git diff --name-only origin/main | terraform-module-resolve --affected ./terraform/production
  continue-on-error: true

- name: Terraform Plan
  if: steps.check.outcome == 'success'
  run: terraform plan
```

### Get All Files for Static Analysis

```bash
terraform-module-resolve --files-only ./terraform/module | xargs tflint
```

### List Changed Module Files

```bash
git diff --name-only HEAD~1 | terraform-module-resolve --files-only --filter-stdin ./terraform/module
```

## License

MIT
