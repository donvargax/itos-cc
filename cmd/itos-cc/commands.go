package main

import (
	"context"
	"slices"
	"strings"

	"github.com/donvargax/kong"

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/slices/units"
)

// The commands under internal/slices declare themselves (ADR-0051): each
// slice's Command names it, gives the line the top-level help lists it by
// and makes its kong struct, and its Ports are the ports its handler is
// built over, which adapters satisfy through methods. This file assembles
// them by hand, units alone so far, until a generator writes it (T-146).

// declared are the commands the slices declare, in their names' order.
var declared = []cli.Declaration{units.Command}

// assembled are the options that add each declared command to c's model,
// each its own fresh command line (Declaration.New), and the providers of
// their handlers over a's ports, which kong calls only when the Run it calls
// asks for their value. kong adds a declared command after commandLine's
// fields, so the model's commands are then put in their names' order
// (byName), the order the top-level help lists them by and the guess for an
// unknown command weighs a tie by.
func assembled(a adapters) []kong.Option {
	options := make([]kong.Option, 0, len(declared)+2)
	for _, d := range declared {
		options = append(options, kong.DynamicCommand(d.Name, d.Help, "", d.New()))
	}
	return append(options, kong.PostBuild(byName),
		// units' handler holds the sources itos-cc.yaml excludes, which its
		// provider reads itself under the Run's context, so a file that
		// cannot be read is config.invalid, never wrapped as a dependent
		// provider's error.
		kong.BindToProvider(func(ctx context.Context) (units.Handler, error) {
			var ports units.Ports = a
			exclude, err := exclusion(ctx, ports.SelectionSettings())
			return units.NewHandler(ports, exclude), err
		}),
	)
}

// byName puts the commands of k's model in their names' order.
func byName(k *kong.Kong) error {
	slices.SortStableFunc(k.Model.Children, func(x, y *kong.Node) int { return strings.Compare(x.Name, y.Name) })
	return nil
}
