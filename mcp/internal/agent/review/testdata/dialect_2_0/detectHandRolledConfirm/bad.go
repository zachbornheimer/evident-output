package fixture

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// run reads the answer from stdin itself while the live region is open.
func run(ctx context.Context, r *evo.Run) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(r.Text(), "delete origin/production-hotfix? [y/N] ")
	answer, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != "y" {
		return evo.Refuse("declined")
	}
	return nil
}
