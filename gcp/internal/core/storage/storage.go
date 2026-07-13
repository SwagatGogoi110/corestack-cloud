package storage

import (
	"sort"
	"sync"
)

type Backend[K comparable, V any] interface {
	Put(key K, value V)
	Get(key K) (V, bool)
	Delete(key K)
	Scan(filter func(K) bool) []V
	Keys() []K
	Clear()
}

type InMemory[K comparable, V any] struct {
	mu sync.RWMutex
	m map[K]V
}

func NewInMenory[K comparable, V any]() *InMemory[K,V] {
	return &InMemory[K, V]{m: make(map[K]V)}
}

func (s *InMemory[K, V]) Put(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

func (s *InMemory[K, V]) Get(key K)(V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	return v, ok
}

func (s *InMemory[K, V]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

func (s *InMemory[K, V]) Scan(filter func(K) bool) []V {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]V, 0)
	for k,v := range s.m {
		if filter == nil || filter(k) {
			out = append(out, v)
		}
	}
	return out
}

func (s *InMemory[K, V]) Keys() []K {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]K, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	return out
}

func (s *InMemory[K, V]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = make(map[K]V)
}

func SortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

