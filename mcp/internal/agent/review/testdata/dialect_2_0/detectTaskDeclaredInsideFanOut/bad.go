package fixture

import (
	"context"
	"os"
	"sync"

	evo "github.com/zachbornheimer/evident-output/v2"
)

type Job struct{ Name string }

func (j Job) Run(ctx context.Context) error { return nil }

func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	jobs := []Job{{Name: "lint"}, {Name: "test"}}
	work := r.Group("jobs")
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			work.Task(j.Name).Define(j.Run)
		}()
	}
	wg.Wait()
	return nil
}
