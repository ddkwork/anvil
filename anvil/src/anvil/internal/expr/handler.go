package expr

type Handler interface {
	Delete(Range)
	Insert(index int, value []byte)
	Display(Range)
	DisplayContents(r Range, prefix string)
	Noop(Range)
	Done()
}
