package mutate

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

const goCoverageEvidenceVersion = 2

func goCoverageEvidence(file *lang.File, unit lang.Unit, blocks []coverage.GoBlock, producer string, inputs map[string]string) *GoCoverageEvidence {
	evidence := &GoCoverageEvidence{
		Version: goCoverageEvidenceVersion, File: project.FromRoot(file.Path),
		Function: unitID(unit.Namespace, unit.Name), Hash: UnitHash(file, unit), Producer: producer,
		Inputs: cloneHashes(inputs), Blocks: []GoCoverageBlock{},
	}
	for _, block := range blocks {
		if !goBlockInUnit(unit.Node, block) || block.Weight <= 0 {
			continue
		}
		evidence.Blocks = append(evidence.Blocks, GoCoverageBlock{
			Span: block.Span, Line: block.Line, Column: block.Column,
			Weight: block.Weight, Covered: block.Covered,
		})
	}
	slices.SortFunc(evidence.Blocks, func(a, b GoCoverageBlock) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		if a.Column != b.Column {
			return a.Column - b.Column
		}
		if a.Span < b.Span {
			return -1
		}
		if a.Span > b.Span {
			return 1
		}
		return 0
	})
	evidence.Complete = len(evidence.Blocks) > 0 || !goHasExecutableStatements(unit.Node)
	return evidence
}

func goBlockInUnit(unit *sitter.Node, block coverage.GoBlock) bool {
	if unit == nil {
		return false
	}
	start, end := unit.StartPosition(), unit.EndPosition()
	row, column := uint(block.Line-1), uint(block.Column-1)
	return (row > start.Row || row == start.Row && column >= start.Column) &&
		(row < end.Row || row == end.Row && column < end.Column)
}

func goHasExecutableStatements(unit *sitter.Node) bool {
	if unit == nil {
		return false
	}
	body := unit.ChildByFieldName("body")
	if body == nil {
		return false
	}
	for i := uint(0); i < body.NamedChildCount(); i++ {
		if body.NamedChild(i).Kind() != "comment" {
			return true
		}
	}
	return false
}

// FreshGoCoverageEvidence binds measured blocks to one frozen admitted
// function without consulting persistent snapshots or the live project root.
func FreshGoCoverageEvidence(root string, admitted FreshUnit, blocks []coverage.GoBlock, producer string, inputs map[string]string) (*GoCoverageEvidence, error) {
	source := filepath.Join(root, filepath.FromSlash(admitted.Path))
	file, err := lang.ParseFile(source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	for _, unit := range file.Units {
		if unit.StartLine != admitted.StartLine || unit.EndLine != admitted.EndLine || UnitHash(file, unit) != admitted.Hash {
			continue
		}
		evidence := &GoCoverageEvidence{
			Version: goCoverageEvidenceVersion, File: admitted.Path,
			Function: unitID(unit.Namespace, unit.Name), Hash: admitted.Hash,
			Producer: producer, Inputs: cloneHashes(inputs), Blocks: []GoCoverageBlock{},
		}
		for _, block := range blocks {
			if goBlockInUnit(unit.Node, block) && block.Weight > 0 {
				evidence.Blocks = append(evidence.Blocks, GoCoverageBlock{
					Span: block.Span, Line: block.Line, Column: block.Column,
					Weight: block.Weight, Covered: block.Covered,
				})
			}
		}
		slices.SortFunc(evidence.Blocks, func(a, b GoCoverageBlock) int {
			if a.Line != b.Line {
				return a.Line - b.Line
			}
			if a.Column != b.Column {
				return a.Column - b.Column
			}
			return strings.Compare(a.Span, b.Span)
		})
		evidence.Complete = len(evidence.Blocks) > 0 || !goHasExecutableStatements(unit.Node)
		return evidence, nil
	}
	return nil, fmt.Errorf("frozen function %s at %s:%d no longer matches its admitted identity", admitted.Function, admitted.Path, admitted.StartLine)
}
