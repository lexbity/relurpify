package capability

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestAllCapabilitySnapshots_IncludesCallable(t *testing.T) {
	reg := NewCapabilityRegistry()
	desc := core.CapabilityDescriptor{
		ID:            "cap:callable",
		Name:          "callable",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	snapshots := reg.AllCapabilitySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Descriptor.ID != desc.ID {
		t.Fatalf("expected descriptor %q, got %q", desc.ID, snapshots[0].Descriptor.ID)
	}
	if snapshots[0].Exposure != core.CapabilityExposureInspectable {
		t.Fatalf("expected inspectable exposure, got %q", snapshots[0].Exposure)
	}
}

func TestAllCapabilitySnapshots_IncludesHidden(t *testing.T) {
	reg := NewCapabilityRegistry()
	desc := core.CapabilityDescriptor{
		ID:            "cap:hidden",
		Name:          "hidden",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	if err := reg.RegisterCapability(desc); err != nil {
		t.Fatalf("register capability: %v", err)
	}
	reg.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{Name: desc.Name},
		Access:   core.CapabilityExposureHidden,
	}})

	if got := reg.AllCapabilities(); len(got) != 0 {
		t.Fatalf("expected hidden capability to be omitted from AllCapabilities, got %d", len(got))
	}

	snapshots := reg.AllCapabilitySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Descriptor.ID != desc.ID {
		t.Fatalf("expected descriptor %q, got %q", desc.ID, snapshots[0].Descriptor.ID)
	}
	if snapshots[0].Exposure != core.CapabilityExposureHidden {
		t.Fatalf("expected hidden exposure, got %q", snapshots[0].Exposure)
	}
}

func TestAllCapabilitySnapshots_Empty(t *testing.T) {
	reg := NewCapabilityRegistry()
	snapshots := reg.AllCapabilitySnapshots()
	if len(snapshots) != 0 {
		t.Fatalf("expected empty snapshots, got %d", len(snapshots))
	}
}

func TestAllCapabilitySnapshots_DelegateRegistry(t *testing.T) {
	reg := NewCapabilityRegistry()
	visible := core.CapabilityDescriptor{
		ID:            "cap:visible",
		Name:          "visible",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	hidden := core.CapabilityDescriptor{
		ID:            "cap:hidden",
		Name:          "delegate-hidden",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
	}
	if err := reg.RegisterCapability(visible); err != nil {
		t.Fatalf("register visible capability: %v", err)
	}
	if err := reg.RegisterCapability(hidden); err != nil {
		t.Fatalf("register hidden capability: %v", err)
	}
	reg.AddExposurePolicies([]core.CapabilityExposurePolicy{{
		Selector: core.CapabilitySelector{ID: hidden.ID},
		Access:   core.CapabilityExposureHidden,
	}})

	scoped := reg.WithAllowlist([]string{hidden.ID})
	snapshots := scoped.AllCapabilitySnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 scoped snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Descriptor.ID != hidden.ID {
		t.Fatalf("expected hidden capability in scoped snapshot, got %q", snapshots[0].Descriptor.ID)
	}
	if snapshots[0].Exposure != core.CapabilityExposureHidden {
		t.Fatalf("expected hidden exposure in scoped snapshot, got %q", snapshots[0].Exposure)
	}
}

func TestAllCapabilitySnapshots_ConcurrentAccess(t *testing.T) {
	reg := NewCapabilityRegistry()
	const total = 32
	errCh := make(chan error, total*2)
	done := make(chan struct{})

	for i := 0; i < total; i++ {
		go func(i int) {
			desc := core.CapabilityDescriptor{
				ID:            "cap:concurrent:" + string(rune('a'+i)),
				Name:          "concurrent",
				Kind:          core.CapabilityKindTool,
				RuntimeFamily: core.CapabilityRuntimeFamilyProvider,
			}
			if err := reg.RegisterCapability(desc); err != nil {
				errCh <- err
				return
			}
			_ = reg.AllCapabilitySnapshots()
			errCh <- nil
		}(i)
	}

	go func() {
		for i := 0; i < total; i++ {
			_ = reg.AllCapabilitySnapshots()
		}
		close(done)
	}()

	for i := 0; i < total; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent register failed: %v", err)
		}
	}
	<-done
}
