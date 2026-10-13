package cli

// Selection is the command line every command selects its files by: the
// files git reports as changed, and the paths, files, directories or
// fragments of one, given as its arguments. A command's kong struct embeds
// it as its first field, so --changed leads its help's options as it always
// has and the command's own flags follow; the arguments end the usage line
// wherever the struct holds them. Its tags need no import of kong. A slice
// copies its values into its own Query, which keeps its own Changed and
// Paths.
type Selection struct {
	Changed bool     `help:"only files git reports as added or modified"`
	Paths   []string `arg:"" optional:"" name:"path" help:"files, directories, or fragments of a path under the working directory"`
}
