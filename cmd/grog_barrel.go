package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jpappel/grog_barrel/pkg/server"
	"github.com/jpappel/grog_barrel/pkg/util"
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	hostname := flag.String("hostname", "localhost", "hostname to listen on")
	loglvl := flag.String("l", "warn", "log level (debug, info, warn, error)")
	socksrv := flag.Bool("sockserver", false, "EXPERIMENTAL: enable unix socket server")
	sockBaseDir := flag.String("sock-base-dir", "/tmp/grogbarrel", "base directory for socket server")

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

	baseCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *socksrv {
		pid := os.Getpid()
		if _, err := os.Stat(*sockBaseDir); errors.Is(err, fs.ErrNotExist) {
			err := os.Mkdir(*sockBaseDir, 0755)
			if err != nil && errors.Is(err, fs.ErrExist) {
				logger.Error("Error occured while creating base dir")
				panic(err)
			}

			file, err := os.Create(*sockBaseDir + "/pid")
			if err != nil {
				panic(err)
			}
			fmt.Fprint(file, pid)
		} else if err != nil {
			panic(err)
		} else {
			if _, err = os.Stat(fmt.Sprint("/proc/", pid)); errors.Is(err, fs.ErrExist) {
				panic(fmt.Sprint("A grogbarrel server is already running with pid:", pid))
			}
		}
		defer os.RemoveAll(*sockBaseDir)

		logger.Info("Starting socket server")
		sockServer := server.NewSockServer(*sockBaseDir, logger)
		go sockServer.Run(baseCtx)
	}

	srv := http.Server{Addr: addr, Handler: server.New(logger)}
	go func() {
		logger.Info("Starting server", slog.String("bindAddress", addr))
		if err := srv.ListenAndServe(); err != http.ErrServerClosed && err != nil {
			logger.Error("Server error", slog.String("err", err.Error()))
		}
	}()

	<-baseCtx.Done()

	logger.Info("Shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Error shutting down server", slog.String("err", err.Error()))
	} else {
		logger.Info("Server shutdown succesfully")
	}
}
