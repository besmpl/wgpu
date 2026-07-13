package hal

import "testing"

func TestMaterialPageOptionalInterfacesAreNarrow(t *testing.T) {
	var provider MaterialPageProvider
	var binder MaterialPageRenderPassEncoder
	if provider != nil || binder != nil {
		t.Fatal("optional extension interfaces should be nil until a backend opts in")
	}
}
