// Command baseline is architecture's import-probe baseline: byte-for-byte
// the same program as ./candidate except it imports nothing from evo. See
// candidate/main.go for the comparison this pair supports.
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Println(runtime.NumGoroutine())
}
