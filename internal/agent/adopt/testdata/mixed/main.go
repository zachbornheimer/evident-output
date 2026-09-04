// Fixture for adopt.Inventory (internal/agent/adopt/adopt_test.go). This is
// deliberately the *pre-adoption* shape the tool is meant to flag — a
// fmt.Printf/log mix plus a manual spinner import — never real production
// code. Do not "fix" these call sites; the golden pins their exact
// line/pattern output.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/briandowns/spinner"
)

func main() {
	s := spinner.New(spinner.CharSets[9], 0)
	s.Start()
	defer s.Stop()

	fmt.Printf("processing %d items\n", 3)
	log.Println("starting work")

	if os.Getenv("REQUIRED") == "" {
		log.Fatal("REQUIRED is not set")
	}
}
