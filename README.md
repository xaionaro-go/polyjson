# `polyjson`

[![Go Reference](https://pkg.go.dev/badge/github.com/xaionaro-go/polyjson.svg)](https://pkg.go.dev/github.com/xaionaro-go/polyjson)
[![Go Report Card](https://goreportcard.com/badge/github.com/xaionaro-go/polyjson)](https://goreportcard.com/report/github.com/xaionaro-go/polyjson)
[![Go Version](https://img.shields.io/github/go-mod/go-version/xaionaro-go/polyjson)](https://golang.org/dl/)

`polyjson` is just a JSON serde which can construct your objects for you.

For example:
```go
    // register a type

	type myFancyStruct struct {
		A int
	}

	polyjson.RegisterType(myFancyStruct{})

	// serialize:

	b, err := polyjson.MarshalWithTypeIDs(map[string]any{"B": myFancyStruct{A: 1}}, polyjson.TypeRegistry())
	if err != nil {
		panic(err)
	}

	// / deserialization
	var cpy map[string]any
	err = polyjson.UnmarshalWithTypeIDs(b, &cpy, polyjson.TypeRegistry())
	if err != nil {
		panic(err)

	}

	if cpy["B"].(myFancyStruct).A != 1 {
		panic("unexpected value")
	}
```
As you can see in this example, it automatically constructed `myFancyStruct` inside `cpy`.