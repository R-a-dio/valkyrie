package util

import (
	"sync"
	"sync/atomic"
)

type Map[K, V any] struct {
	m sync.Map
}

func (m *Map[K, V]) Clear() {
	m.m.Clear()
}

func (m *Map[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	return m.m.CompareAndDelete(key, old)
}

func (m *Map[K, V]) CompareAndSwap(key K, old V, new V) bool {
	return m.m.CompareAndSwap(key, old, new)
}

func (m *Map[K, V]) Delete(key K) {
	m.m.Delete(key)
}

func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	v, ok := m.m.Load(key)
	if !ok || v == nil {
		return *new(V), false
	}
	return v.(V), true
}

func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	v, loaded := m.m.LoadAndDelete(key)
	if v == nil {
		return *new(V), loaded
	}
	return v.(V), loaded
}

func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	a, loaded := m.m.LoadOrStore(key, value)
	if a == nil {
		return *new(V), loaded
	}
	return a.(V), loaded
}

func (m *Map[K, V]) Range(fn func(key K, value V) bool) {
	m.m.Range(func(key, value any) bool {
		return fn(key.(K), value.(V))
	})
}

func (m *Map[K, V]) Store(key K, value V) {
	m.m.Store(key, value)
}

func (m *Map[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	p, loaded := m.m.Swap(key, value)
	if p == nil {
		return *new(V), loaded
	}
	return p.(V), loaded
}

func NewTypedValue[T any](initial T) *TypedValue[T] {
	t := new(TypedValue[T])
	t.Store(initial)
	return t
}

type TypedValue[T any] struct {
	v atomic.Pointer[T]
}

func (tv *TypedValue[T]) Store(new T) {
	tv.v.Store(&new)
}

func (tv *TypedValue[T]) Load() T {
	lv := tv.v.Load()
	if lv == nil {
		return *new(T)
	}
	return *lv
}

func (tv *TypedValue[T]) Swap(newv T) (old T) {
	sv := tv.v.Swap(&newv)
	if sv == nil {
		return *new(T)
	}
	return *sv
}
