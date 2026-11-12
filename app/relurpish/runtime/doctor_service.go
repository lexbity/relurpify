package runtime

import (
	"context"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

// DoctorService diagnoses boot failure without constructing an execution
// runtime, starting its services, or requiring a usable model.
type DoctorService struct {
	config    Config
	secrets   config.Secrets
	bootError error
}

func NewDoctorService(cfg Config, secrets config.Secrets, bootError error) *DoctorService {
	return &DoctorService{config: cfg, secrets: secrets, bootError: bootError}
}

func (s *DoctorService) BuildDoctorReport(ctx context.Context) DoctorReport {
	report := BuildDoctorReport(ctx, s.config, s.secrets)
	if s.bootError != nil {
		report.Dependencies = append(report.Dependencies, DependencyStatus{
			Name: "execution_boot", Blocking: true, Details: s.bootError.Error(),
		})
	}
	return report
}
