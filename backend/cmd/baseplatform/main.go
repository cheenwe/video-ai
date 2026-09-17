package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"
	"baseplatform/internal/config"
	"baseplatform/internal/database"
	"baseplatform/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = database.BootstrapAdmin(db); err != nil {
		log.Fatal(err)
	}
	front := os.Getenv("BASE_FRONTEND_DIR")
	if front == "" {
		front = "../frontend/dist"
	}
	app, err := server.NewApplication(cfg, db, front)
	if err != nil {
		log.Fatal(err)
	}
	if err = app.Start(); err != nil {
		log.Fatal(err)
	}
	defer app.Stop()
	srv := &http.Server{Addr: net.JoinHostPort(cfg.Host, cfg.Port), Handler: app.Router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(c); err != nil {
			log.Printf("HTTP shutdown: %v", err)
			_ = srv.Close()
		}
	}()
	log.Printf("Base Platform listening at http://%s", srv.Addr)
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP server: %v", err)
	}
	stop()
	<-shutdownDone
}
