package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/update"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
)

const updateUsage = "usage: protonman update [version]"

func runUpdateCommand(ctx context.Context, args []string) (bool, error) {
	if len(args) == 0 || args[0] != "update" {
		return false, nil
	}
	pinned := ""
	switch rest := args[1:]; {
	case len(rest) == 0:
	case len(rest) == 1 && (rest[0] == "-h" || rest[0] == "--help"):
		fmt.Fprintln(os.Stdout, updateUsage)
		return true, nil
	case len(rest) == 1 && !strings.HasPrefix(rest[0], "-"):
		pinned = strings.TrimSpace(rest[0])
	default:
		return true, errors.New(updateUsage)
	}
	if pinned != "" {
		if _, err := update.ValidateTag(pinned); err != nil {
			return true, err
		}
	}
	if err := runUpdate(ctx, pinned); err != nil {
		return true, err
	}
	return true, nil
}

func runUpdate(ctx context.Context, pinned string) error {
	result, err := update.Run(ctx, update.Config{RequestedTag: pinned})
	if err != nil {
		return err
	}
	if result.Status == update.StatusUpToDate {
		fmt.Fprintf(os.Stdout, "%s %s (up to date)\n", buildinfo.Name, result.Current)
		return nil
	}
	fmt.Fprintf(os.Stdout, "Updated %s %s -> %s (%s)\n", buildinfo.Name, result.Previous, result.Current, result.Location)
	return nil
}
