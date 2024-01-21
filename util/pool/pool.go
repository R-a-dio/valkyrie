package pool

import "sync"

// Pool is a sync.Pool wrapped for type-safety
type Pool[T any] struct {
	p sync.Pool
}

func NewPool[T any](newFn func() T) *Pool[T] {
	return &Pool[T]{
		sync.Pool{
			New: func() any { return newFn() },
		},
	}
}

func (p *Pool[T]) Get() T {
	return p.p.Get().(T)
}

func (p *Pool[T]) Put(v T) {
	p.p.Put(v)
}

type Resetable interface {
	Reset()
}

// ResetPool is a sync.Pool wrapped with a generic Resetable interface, the pool calls
// Reset before returning an item to the pool.
type ResetPool[T Resetable] struct {
	*Pool[T]
}

func NewResetPool[T Resetable](newFn func() T) *ResetPool[T] {
	return &ResetPool[T]{NewPool(newFn)}
}

func (p *ResetPool[T]) Put(v T) {
	v.Reset()
	p.p.Put(v)
}
