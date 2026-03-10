package admin

import "time"

type service struct {
	cfg ServiceConfig
}

func NewService(cfg ServiceConfig) AdminService {
	if cfg.StartedAt.IsZero() {
		cfg.StartedAt = time.Now().UTC()
	}
	if cfg.Partition == "" {
		cfg.Partition = "local"
	}
	return &service{cfg: cfg}
}
