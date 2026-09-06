package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/zyvorai/fleet/internal/auth"
	"github.com/zyvorai/fleet/internal/model"
	"github.com/zyvorai/fleet/internal/server"
	"github.com/zyvorai/fleet/internal/store"
)

var version = "dev"

func main() {
	listen := flag.String("listen", envOr("ZYVOR_FLEET_LISTEN", ":8080"), "HTTP listen address")
	data := flag.String("data", envOr("ZYVOR_FLEET_DATA", "./data/state.json"), "persistent state file")
	demo := flag.Bool("demo", os.Getenv("ZYVOR_FLEET_DEMO") == "1", "enable explicit demo defaults")
	tlsCert := flag.String("tls-cert", os.Getenv("ZYVOR_FLEET_TLS_CERT"), "TLS certificate path")
	tlsKey := flag.String("tls-key", os.Getenv("ZYVOR_FLEET_TLS_KEY"), "TLS private key path")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	st, err := store.Open(*data)
	fatalIf(logger, err)
	adminEmail := envOr("ZYVOR_FLEET_ADMIN_EMAIL", "admin@zyvor.local")
	adminPassword := os.Getenv("ZYVOR_FLEET_ADMIN_PASSWORD")
	if adminPassword == "" && *demo {
		adminPassword = "zyvor-fleet-demo"
	}
	if adminPassword == "" && len(st.Snapshot().Users) == 0 {
		logger.Error("first start requires ZYVOR_FLEET_ADMIN_PASSWORD (10+ chars), or --demo for local evaluation")
		os.Exit(2)
	}
	if adminPassword != "" {
		fatalIf(logger, ensureAdmin(st, adminEmail, adminPassword))
	}

	secret, ephemeral, err := sessionSecret(*demo)
	fatalIf(logger, err)
	if ephemeral {
		logger.Warn("using ephemeral session secret; sessions will be invalid after restart; set ZYVOR_FLEET_SESSION_SECRET in production")
	}
	sessions, err := auth.NewSessionManager(secret, 12*time.Hour)
	fatalIf(logger, err)
	if *demo {
		token, err := ensureDemoEnrollment(st)
		fatalIf(logger, err)
		logger.Warn("DEMO MODE enabled", "admin", adminEmail, "password", adminPassword, "enrollment_token", token)
	}

	h := server.New(st, sessions, server.Config{SecureCookies: (*tlsCert != "" && *tlsKey != "") || os.Getenv("ZYVOR_FLEET_SECURE_COOKIES") == "1", Logger: logger}).Handler()
	httpServer := &http.Server{Addr: *listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("Zyvor Fleet control plane started", "version", version, "listen", *listen)
		if *tlsCert != "" || *tlsKey != "" {
			if *tlsCert == "" || *tlsKey == "" {
				errCh <- fmt.Errorf("both TLS cert and key are required")
				return
			}
			errCh <- httpServer.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			errCh <- httpServer.ListenAndServe()
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		logger.Info("shutdown requested", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}
}

func ensureAdmin(st *store.Store, email, password string) error {
	state := st.Snapshot()
	for _, u := range state.Users {
		if strings.EqualFold(u.Email, email) {
			return nil
		}
	}
	h, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return st.Update(func(s *model.State) error {
		s.Users = append(s.Users, model.User{ID: "usr_admin", Email: strings.ToLower(email), Name: "Fleet Administrator", Role: model.RoleAdmin, PasswordHash: h, CreatedAt: time.Now().UTC()})
		return nil
	})
}
func ensureDemoEnrollment(st *store.Store) (string, error) {
	const plain = "zf_enroll_demo-local-only"
	state := st.Snapshot()
	hash := auth.SHA256Token(plain)
	for _, t := range state.EnrollmentTokens {
		if t.TokenHash == hash {
			return plain, nil
		}
	}
	err := st.Update(func(s *model.State) error {
		s.EnrollmentTokens = append(s.EnrollmentTokens, model.EnrollmentToken{ID: "enr_demo", Name: "Demo enrollment", TokenHash: hash, CreatedAt: time.Now().UTC(), MaxUses: 1000})
		return nil
	})
	return plain, err
}
func sessionSecret(demo bool) ([]byte, bool, error) {
	raw := os.Getenv("ZYVOR_FLEET_SESSION_SECRET")
	if raw != "" {
		if b, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(b) >= 32 {
			return b, false, nil
		}
		if len(raw) >= 32 {
			return []byte(raw), false, nil
		}
		return nil, false, fmt.Errorf("ZYVOR_FLEET_SESSION_SECRET must contain at least 32 bytes")
	}
	b, err := auth.RandomSecret(48)
	return b, true, err
}
func envOr(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func fatalIf(logger *slog.Logger, err error) {
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
}
