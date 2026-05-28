package euclobaselinechat

type Store interface {
	Save(key, value string) error
	Load(key string) (string, error)
}

type MemoryStore struct{ data map[string]string }

func (s *MemoryStore) Save(key, value string) error {
	s.data[key] = value
	return nil
}

func (s *MemoryStore) Load(key string) (string, error) {
	return s.data[key], nil
}
