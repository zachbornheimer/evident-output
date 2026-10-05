package scoreboard

import (
	"fmt"
	"os"

	"github.com/zachbornheimer/evident-output/eval/runner"
)

// ReadRecordFiles parses every transcript file and concatenates the records.
func ReadRecordFiles(paths []string) ([]runner.Record, error) {
	var all []runner.Record
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open transcript %s: %w", path, err)
		}
		records, err := ReadRecords(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("read transcript %s: %w", path, err)
		}
		all = append(all, records...)
	}
	return all, nil
}
