package oapi

import "testing"

func TestTypesCompile(t *testing.T) {
	var _ Disk
	var _ Pool
	var _ Dataset
	var _ Snapshot
	var _ Job
}
