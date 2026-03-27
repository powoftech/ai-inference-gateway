package routing

import (
	"errors"
	"math/rand"
	"sync"
)

type LoadBalancer struct {
	backends []*Backend
	mu       sync.RWMutex
}

func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		backends: make([]*Backend, 0),
	}
}

func (lb *LoadBalancer) AddBackend(b *Backend) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.backends = append(lb.backends, b)
}

// SelectWeightedBackend picks the fastest healthy backend using CDF logic
func (lb *LoadBalancer) SelectWeightedBackend() (*Backend, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var totalWeight float64
	weights := make([]float64, len(lb.backends))

	for i, b := range lb.backends {
		w := b.GetWeight()
		weights[i] = w
		totalWeight += w
	}

	if totalWeight == 0 {
		return nil, errors.New("no healthy backends available (all circuits open)")
	}

	// Pick a random number between 0 and totalWeight
	r := rand.Float64() * totalWeight

	// Binary/Linear search for the selected backend
	for i, w := range weights {
		r -= w
		if r <= 0 {
			return lb.backends[i], nil
		}
	}

	return lb.backends[len(lb.backends)-1], nil
}
