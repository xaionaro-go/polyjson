package polyjson_test

import (
	"fmt"

	"github.com/xaionaro-go/polyjson"
)

func ExampleUnmarshalWithTypeIDs() {
	type myFancyStruct struct {
		A int
	}

	polyjson.RegisterType(myFancyStruct{})

	// serialize:

	b, err := polyjson.MarshalWithTypeIDs(map[string]any{"B": myFancyStruct{A: 1}}, polyjson.TypeRegistry())
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))

	// deserialize:

	var cpy map[string]any
	err = polyjson.UnmarshalWithTypeIDs(b, &cpy, polyjson.TypeRegistry())
	if err != nil {
		panic(err)
	}

	if cpy["B"].(myFancyStruct).A != 1 {
		panic("unexpected value")
	}

	// Output:
	// {"B":{"./polyjson_test.myFancyStruct":{"A":1}}}
}
