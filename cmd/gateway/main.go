package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"dnd-gateway/internal/config"
	"dnd-gateway/internal/diceclient"
	"dnd-gateway/internal/dicepb"
	"dnd-gateway/internal/diceserver"
	"dnd-gateway/internal/diceupstream"
	"dnd-gateway/internal/dnd"
	"dnd-gateway/internal/httpapi"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fs := flag.NewFlagSet("dnd-gateway", flag.ContinueOnError)
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "all, http, or grpc")
	fs.StringVar(&cfg.HTTPAddr, "http", cfg.HTTPAddr, "HTTP listen address")
	fs.StringVar(&cfg.GRPCAddr, "grpc", cfg.GRPCAddr, "dice gRPC listen or dial address")
	fs.StringVar(&cfg.DNDBaseURL, "dnd", cfg.DNDBaseURL, "D&D 5e API base URL")
	fs.StringVar(&cfg.DNDVersion, "dnd-version", cfg.DNDVersion, "D&D API version: 2014 or 2024")
	fs.StringVar(&cfg.DiceBaseURL, "dice", cfg.DiceBaseURL, "public dice API base URL")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Fatal(err)
	}
}

func grpcTarget(addr string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	return "dns:///" + addr
}

func run(ctx context.Context, cfg config.Config, logger *log.Logger) error {
	var grpcServer *grpc.Server
	if cfg.Mode == config.ModeAll || cfg.Mode == config.ModeGRPC {
		upstream, err := diceupstream.New(cfg.DiceBaseURL)
		if err != nil {
			return err
		}
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			return fmt.Errorf("listen grpc: %w", err)
		}
		grpcServer = grpc.NewServer(dicepb.ServerOptions()...)
		dicepb.RegisterDiceServiceServer(grpcServer, diceserver.New(upstream))
		go func() {
			logger.Printf("dice gRPC listening on %s", lis.Addr())
			if err := grpcServer.Serve(lis); err != nil {
				logger.Printf("grpc server: %v", err)
			}
		}()
		defer grpcServer.GracefulStop()
	}

	if cfg.Mode == config.ModeGRPC {
		<-ctx.Done()
		logger.Printf("shutting down")
		return nil
	}

	dialOpts := append(dicepb.DialOptions(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(grpcTarget(cfg.GRPCAddr), dialOpts...)
	if err != nil {
		return fmt.Errorf("dial dice grpc: %w", err)
	}
	defer conn.Close()

	rules, err := dnd.New(cfg.DNDBaseURL, cfg.DNDVersion)
	if err != nil {
		return err
	}
	handler := httpapi.New(rules, diceclient.New(dicepb.NewDiceServiceClient(conn)), logger)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("HTTP gateway listening on http://%s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Printf("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
