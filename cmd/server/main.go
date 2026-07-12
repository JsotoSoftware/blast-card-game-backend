package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"exploding-game/server/internal/room"
	"exploding-game/server/internal/transport"
)

const defaultPort = "8080"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := serverPort()
	addr := ":" + port

	roomManager := room.NewManager()
	wsHandler := transport.NewWebSocketHandler(roomManager, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("GET /ws", wsHandler)

	server := &http.Server{
		Addr:              addr,
		Handler:           requestLogger(mux, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdownErr := make(chan error, 1)
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop

		logger.Info("shutdown signal received")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		shutdownErr <- server.Shutdown(ctx)
	}()

	logger.Info("server listening", "addr", addr, "port", port)

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}

	if err := <-shutdownErr; err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func requestLogger(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(recorder, r)

		logger.Info(
			"request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

func serverPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		return defaultPort
	}
	if _, err := strconv.Atoi(port); err != nil {
		slog.Warn("invalid PORT value; using default", "value", port, "default", defaultPort)
		return defaultPort
	}
	return port
}
