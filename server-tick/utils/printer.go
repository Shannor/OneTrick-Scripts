package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// PrettyPrint pretty-prints a struct or map in JSON format.
func PrettyPrint(input interface{}) {
	bytes, err := json.MarshalIndent(input, "", "    ")
	if err != nil {
	}
	fmt.Println(string(bytes))
}

// PrintStructKV prints the keys and values of a struct.
func PrintStructKV(input interface{}) {
	val := reflect.ValueOf(input)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		fmt.Println("Provided input is not a struct")
		return
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		value := val.Field(i)
		fmt.Printf("%s: %v\n", field.Name, value.Interface())
	}
}
