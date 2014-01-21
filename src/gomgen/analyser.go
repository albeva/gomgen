package gomgen

type Analyzer interface {
	Analyze(gen *Generator)
}
