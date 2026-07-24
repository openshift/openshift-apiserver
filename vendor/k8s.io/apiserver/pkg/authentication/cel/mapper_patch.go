package cel

import (
	"context"

	"github.com/google/cel-go/common/types/traits"
)

type Mapper struct {
	*mapper
}

func NewMapper(compilationResults []CompilationResult) *Mapper {
	return &Mapper{
		mapper: &mapper{
			compilationResults: compilationResults,
		},
	}
}

func (m *Mapper) Eval(ctx context.Context, input VarNameActivation) ([]EvaluationResult, error) {
	return m.mapper.eval(ctx, input.varNameActivation)
}

type VarNameActivation struct {
	*varNameActivation
}

func NewVarNameActivation(name string, value traits.Mapper) VarNameActivation {
	return VarNameActivation{
		varNameActivation: &varNameActivation{
			name: name,
			value: value,
		},
	}
}
