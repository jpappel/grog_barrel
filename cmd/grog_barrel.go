package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/jpappel/grog_barrel/pkg/server"
	"github.com/jpappel/grog_barrel/pkg/util"
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	hostname := flag.String("hostname", "localhost", "hostname to listen on")
	loglvl := flag.String("l", "warn", "log level (debug, info, warn, error)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "grogbarrel", util.ServerVersion.String())
	}
	flag.Parse()

	loggerOpts := new(slog.HandlerOptions)
	switch *loglvl {
	case "debug":
		loggerOpts.Level = slog.LevelDebug
		loggerOpts.AddSource = true
	case "info":
		loggerOpts.Level = slog.LevelInfo
	case "warn":
		loggerOpts.Level = slog.LevelWarn
	case "error":
		loggerOpts.Level = slog.LevelError
	default:
		panic(fmt.Sprintf("Unkown log level %s", *loglvl))
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, loggerOpts))

	addr := fmt.Sprintf("%s:%d", *hostname, *port)

	mux := server.New(logger)
	logger.Info("Starting server", slog.String("bindAddress", addr))
	logger.Info(http.ListenAndServe(addr, mux).Error())
}
