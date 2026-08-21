package manager

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed ui/*
var uiFS embed.FS

// Options 配置 manager HTTP 服务。
type Options struct {
	// Addr 监听地址，空值时使用 :8080。
	Addr string
	// ValidateBuild 在结构校验与 ValidatePlan 通过后可选执行完整 build。
	// 宿主可在此调用 build.Build[T] 做更严格的校验。
	ValidateBuild func(ctx context.Context, doc Document) error
}

// New 返回包含 UI 与 /api/* 的 http.Handler。
func New(opts Options) (http.Handler, error) {
	ui, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return nil, err
	}
	srv := &server{validateBuild: opts.ValidateBuild}
	mux := http.NewServeMux()
	srv.registerRoutes(mux)
	mux.Handle("/", http.FileServer(http.FS(ui)))
	return withLogging(mux), nil
}

// Run 启动 HTTP 服务并在收到 SIGINT/SIGTERM 后优雅退出。
func Run(opts Options) error {
	if opts.Addr == "" {
		opts.Addr = ":8080"
	}
	handler, err := New(opts)
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              opts.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("pluginkit manager listening", "addr", opts.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("manager listen failed", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("manager request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}
