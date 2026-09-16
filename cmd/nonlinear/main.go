package main

import (
	"crypto/subtle"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/johnspence0212/NonLinear/internal/api"
	mcpserver "github.com/johnspence0212/NonLinear/internal/mcp"
	"github.com/johnspence0212/NonLinear/internal/store"
	"github.com/johnspence0212/NonLinear/internal/version"
	"github.com/johnspence0212/NonLinear/web"
)

func main() {
	addr := flag.String("addr", envOr("ADDR", ""), "listen address, e.g. :3333")
	dataDir := flag.String("data", envOr("DATA_DIR", "./data"), "directory for db.json")
	token := flag.String("token", envOr("NL_TOKEN", ""), "optional bearer token for /api and /mcp")
	openUI := flag.Bool("open", envBool("NL_OPEN"), "open a dedicated Chromium app window")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	listen := *addr
	if listen == "" {
		port := envOr("PORT", "3333")
		listen = ":" + port
	}

	st, err := store.Open(filepath.Join(*dataDir, "db.json"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	mux := http.NewServeMux()
	apiHandler := &api.Handler{Store: st}
	apiHandler.Register(mux)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpserver.New(st)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)

	static := fs.FS(web.FS)
	fileServer := http.FileServer(http.FS(static))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/"), "/") {
			if _, err := fs.Stat(static, strings.TrimPrefix(r.URL.Path, "/")); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if r.URL.Path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})

	handler := withCORS(withAuth(*token, mux))
	fmt.Printf("nonlinear %s  ui  http://127.0.0.1%s\n", version.Version, listen)
	fmt.Printf("nonlinear %s  mcp http://127.0.0.1%s/mcp\n", version.Version, listen)
	fmt.Printf("nonlinear %s  data %s\n", version.Version, filepath.Join(*dataDir, "db.json"))
	if *openUI {
		openAppWindow("http://127.0.0.1" + listen)
	}
	if err := http.ListenAndServe(listen, handler); err != nil {
		log.Fatal(err)
	}
}

func withAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isStatic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Mcp-Session-Id, MCP-Protocol-Version")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isStatic(path string) bool {
	if path == "/" || path == "/index.html" {
		return true
	}
	switch path {
	case "/app.css", "/app.js", "/manifest.json", "/sw.js", "/icon-192.png", "/icon-512.png":
		return true
	default:
		return false
	}
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
