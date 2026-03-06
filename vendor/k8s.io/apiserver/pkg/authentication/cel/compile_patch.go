package cel

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"k8s.io/apimachinery/pkg/util/version"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"
)

type EnvironmentSet struct {
	variableName string
	envOptions   []cel.EnvOption
	declTypes    []*apiservercel.DeclType
}

func (es *EnvironmentSet) Build(baseEnv *environment.EnvSet) (*environment.EnvSet, error) {
	return baseEnv.Extend(environment.VersionedOptions{
		IntroducedVersion: version.MajorMinor(1, 0),
		EnvOptions:        es.envOptions,
		DeclTypes:         es.declTypes,
	})
}

// NewEnvironmentSet returns a *EnvironmentSet that is used for configuring an ExtendableCompiler
// with a new CEL environment variable. The variable name parameter is an arbitrary string
// representing the new environment variable. It must not be empty.
func NewEnvironmentSet(variableName string, options []cel.EnvOption, declTypes []*apiservercel.DeclType) *EnvironmentSet {
	if len(variableName) == 0 {
		panic("variable name must not be an empty string")
	}

	return &EnvironmentSet{
		variableName: variableName,
		envOptions:   options,
		declTypes:    declTypes,
	}
}

// ExtendableCompiler is an extension of the baseline compiler
// that allows for extending the capabilities of the baseline compiler.
type ExtendableCompiler struct {
	*compiler
}

func (ec *ExtendableCompiler) Compile(accessor ExpressionAccessor, variableName string) (CompilationResult, error) {
	return ec.compiler.compile(accessor, variableName)
}

func NewExtendableCompiler(baseEnv *environment.EnvSet, additionalEnvSets ...*EnvironmentSet) *ExtendableCompiler {
	compiler := &compiler{
		varEnvs: mustBuildEnvs(baseEnv),
	}

	for _, envSet := range additionalEnvSets {
		builtEnvSet, err := envSet.Build(baseEnv)
		if err != nil {
			panic(fmt.Sprintf("building environment set for variable %q: %v", envSet.variableName, err))
		}

		compiler.varEnvs[envSet.variableName] = builtEnvSet
	}

	return &ExtendableCompiler{
		compiler: compiler,
	}
}
