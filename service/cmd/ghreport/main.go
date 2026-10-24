// Команда ghreport строит сводку по открытым issue репозиториев GitHub.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"ghreport/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.Run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
