package evo_test

import (
	"os"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/manifest/manifesttest"
)

func TestMain(m *testing.M) { os.Exit(manifesttest.Run(m)) }
