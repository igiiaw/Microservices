package idempotency

import "sync"

// MemoryStore tracks processed messages and retry counts in memory.
type MemoryStore struct {
	mu       sync.RWMutex
	seen     map[string]struct{}
	attempts map[string]int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		seen:     make(map[string]struct{}),
		attempts: make(map[string]int),
	}
}

// IsDuplicate returns true if the message was already successfully processed.
func (s *MemoryStore) IsDuplicate(messageID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.seen[messageID]
	return exists
}

// MarkProcessed flags a message as done and clears its retry history to save memory.
func (s *MemoryStore) MarkProcessed(messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[messageID] = struct{}{}
	delete(s.attempts, messageID)
}

// IncrementAttempts bumps and returns the retry count for a message.
func (s *MemoryStore) IncrementAttempts(messageID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[messageID]++
	return s.attempts[messageID]
}
