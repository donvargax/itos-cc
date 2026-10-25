package cli

import (
	"strings"

	"github.com/donvargax/kong"
)

// DidYouMean starts a fix with what word may have meant among candidates,
// each as name writes it: "Did you mean X? " for one, "Did you mean one of
// X, Y? " for several, in candidates' order, and "" for none. What it names
// is the kong fork's Suggest (decision 62): kong's rule, every candidate
// that starts with word or is within two edits of it, and when that finds
// none, the one candidate all but word's last letter begins, five letters
// at least, such as mutation for mutate. An unknown command and an unknown
// flag name what was meant by it.
func DidYouMean(word string, candidates []string, name func(string) string) string {
	var meant []string
	for _, c := range kong.Suggest(word, candidates) {
		meant = append(meant, name(c))
	}
	switch len(meant) {
	case 0:
		return ""
	case 1:
		return "Did you mean " + meant[0] + "? "
	}
	return "Did you mean one of " + strings.Join(meant, ", ") + "? "
}
