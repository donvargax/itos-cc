package cli

// Declaration is what a slice declares its command by, which cmd assembles
// the one model of every command from (ADR-0051): Name, the word that runs
// it, Help, the line the top-level help lists it by, and New, which makes a
// fresh value of its kong struct, a pointer, for each parser cmd builds, so
// no parse shares the state of another. The handler a command's Run is
// handed is built in cmd from the ports the slice declares beside it, its
// Ports, which cmd's adapters satisfy; a declaration holds no port and
// reads nothing, so declaring a command costs help and version nothing.
type Declaration struct {
	Name string
	Help string
	New  func() any
}
