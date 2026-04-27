package bindings

import "github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"

// SkeletonFromDeclarations builds a fillable bindings skeleton from template declarations.
func SkeletonFromDeclarations(declared map[string]VariableDeclaration) VarsFile {
	variables := make(map[string]VariableBinding, len(declared))
	for _, name := range contractutil.SortedKeys(declared) {
		declaration := declared[name]
		variables[name] = VariableBinding{
			Value:       "",
			Description: declaration.Description,
		}
	}

	return VarsFile{
		SchemaVersion: varsSchemaVersion,
		Variables:     variables,
	}
}
