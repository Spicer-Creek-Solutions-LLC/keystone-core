package scenarios

import (
	"testing"

	"github.com/shawnbutts/keystone-core/test/bootstrap/vm"
)

func TestVMSmokeConfig(t *testing.T) {
	vm.RunVMTests(t, "", []func(*testing.T, vm.Provider, *vm.Config){
		func(t *testing.T, provider vm.Provider, cfg *vm.Config) {
			nodes := provider.ListNodes()
			if len(nodes) == 0 {
				t.Fatal("expected at least one VM node configured")
			}
		},
	})
}
