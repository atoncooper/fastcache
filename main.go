package main

import (
	"fmt"
	"github.com/atoncooper/fastcache/src"
)

func main() {
	cache := src.NewFastCache()
	cache.Set("key1", "value1", 10)
	data, exists := cache.Get("key1")
	fmt.Println(data, exists)
	cache.Delete("key1")
	data, exists = cache.Get("key1")
	fmt.Println(data, exists)
	cache.SetM2One([]string{"key1", "key2"}, "value2", 10)
	data, exists = cache.Get("key1")
	fmt.Println(data, exists)
	data, exists = cache.Get("key2")
	fmt.Println(data, exists)

}
