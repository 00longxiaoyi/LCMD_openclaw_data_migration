package main

import (
	"fmt"
	"os"

	"openclaw_data_migration/internal/migrate"
)

func main() {
	if err := migrate.RunInteractive(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
