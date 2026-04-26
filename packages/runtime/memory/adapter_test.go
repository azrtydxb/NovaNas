package memory_test

import (
	"testing"

	rt "github.com/azrtydxb/novanas/packages/runtime"
	"github.com/azrtydxb/novanas/packages/runtime/conformance"
	"github.com/azrtydxb/novanas/packages/runtime/memory"
)

func TestConformance(t *testing.T) {
	conformance.Run(t, func(_ *testing.T) (rt.Adapter, func()) {
		return memory.New(), func() {}
	})
}
