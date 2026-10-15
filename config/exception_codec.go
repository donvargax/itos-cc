package config

type exceptionWire struct {
	File     string `yaml:"file"`     // slash-separated, from the project root
	Function string `yaml:"function"` // namespace#name
	Hash     string `yaml:"hash"`     // the function's hash, as the snapshot records it
	// LineInFunction is the site's line counted from the function's first
	// line, which is 1.
	LineInFunction int    `yaml:"line_in_function"`
	Column         int    `yaml:"column"`
	Original       string `yaml:"original"`
	Replacement    string `yaml:"replacement"` // empty for a deletion
	Reason         string `yaml:"reason"`
	// LineText is the text of the site's line, less the space around it, as
	// it was when the entry was written: what mutation except --renew
	// matches the line against. Optional: entries written before it have
	// none.
	LineText string `yaml:"line_text,omitempty"`
}
