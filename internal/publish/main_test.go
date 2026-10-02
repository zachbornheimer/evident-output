package publish

import (
	"os"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot/cacherootest"
)

func TestMain(m *testing.M) { os.Exit(cacherootest.Run(m)) }
