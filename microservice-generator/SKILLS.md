---
name: microservice-generator
description: Generate a Go microservice template from DECISIONS.md using a provided microservice name. Use when the user wants to scaffold a new service from the decisions captured in this repository.
---
Generate a Go microservice template from the decisions in DECISIONS.md.

Workflow:
1. If the microservice name is not provided, ask for it.
2. Read DECISIONS.md from the workspace root.
3. Run the local generator at `microservice-generator/generate_template.go` with the microservice name.
4. Generate the service as a sibling directory to the current `ms_template` folder.
5. Run the generator with verification enabled so the generated service is dependency-resolved, unit-tested, and built.
6. Report the output path, verification result, and the key generated files.

Constraints:
- Treat DECISIONS.md as the source of truth for the scaffold.
- Generated code must follow DRY and SOLID principles as defined in DECISIONS.md.
- Do not modify DECISIONS.md while generating a service.
- Preserve existing output directories unless the user explicitly wants overwrite behavior.
- Use the generated microservice name to derive directory name, module name, and display name.
- Treat generation as successful only if the generator reports verification success, unless the user explicitly asks to skip verification.

Example:
- User asks: "Generate service payments-api"
- Run: `go run microservice-generator/generate_template.go --service-name payments-api --verify=true --verify-level quick`
