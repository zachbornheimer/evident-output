package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/apisurface"
)

func cmdContract(args []string) error {
	dir, err := parseContractDir(args)
	if err != nil {
		return err
	}
	report, err := apisurface.CheckDir(dir)
	if err != nil {
		return err
	}
	if report.OK() {
		return nil
	}
	return report
}

func parseContractDir(args []string) (string, error) {
	dir := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--dir":
			if i+1 >= len(args) {
				return "", fmt.Errorf("usage: evident-output contract [--dir PATH]")
			}
			i++
			dir = args[i]
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		default:
			return "", fmt.Errorf("usage: evident-output contract [--dir PATH]")
		}
	}
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return cwd, nil
	}
	return dir, nil
}
