package tools

import (
	"testing"
	"unsafe"
)


func test_cat_eco_loc() uintptr{
	//var buff [32]any
	buff := [32]any{}
	r:=Cat_any_array( buff[:0] , []any{1,2} );
	//r:=cat_any_array_tx( buff[:0] , []any{1,2} );
	//r:=append( buff[:0] , 1,2 );
	//fmt.Println(r);
	return uintptr(unsafe.Pointer(&r[0]))
}
func Test_cat_eco(t *testing.T) {
	r:=[]uintptr{ test_cat_eco_loc(), test_cat_eco_loc() }; 	
	if (r[0]!=r[1]) { t.Fail(); } // Объект не на стеке после Cat_any_array!
}
