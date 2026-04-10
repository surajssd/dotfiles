package runtime

import (
	"testing"
)

func TestRunOptsDefaults(t *testing.T) {
	// Verify zero-value RunOpts is valid.
	opts := RunOpts{}
	if opts.Name != "" {
		t.Errorf("Name = %q, want empty", opts.Name)
	}
	if opts.Detach {
		t.Error("Detach should default to false")
	}
}

func TestContainerInfo(t *testing.T) {
	info := ContainerInfo{Name: "test", Status: "running"}
	if info.Name != "test" || info.Status != "running" {
		t.Errorf("ContainerInfo = %+v", info)
	}
}

func TestPortMapping(t *testing.T) {
	pm := PortMapping{Host: 8080, Container: 80}
	if pm.Host != 8080 || pm.Container != 80 {
		t.Errorf("PortMapping = %+v", pm)
	}
}
