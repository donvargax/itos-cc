package board

type Board[T any] struct{ cells []T }

func New() *Board[int] {
	return &Board[int]{}
}

func (b *Board[T]) Place(v T) {
	add := func() { b.cells = append(b.cells, v) }
	add()
}

func (b Board[T]) size() int { return len(b.cells) }

func helper() {}
