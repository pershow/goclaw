package gateway

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/smallnest/goclaw/internal/logger"
	"go.uber.org/zap"
)

//go:embed dist/control-ui
var controlUIAssets embed.FS

// ServeControlUI 提供 Control UI 静态文件服务
func (s *Server) ServeControlUI(mux *http.ServeMux) error {
	// 获取嵌入的文件系统
	distFS, err := fs.Sub(controlUIAssets, "dist/control-ui")
	if err != nil {
		return err
	}

	// 创建文件服务器
	fileServer := http.FileServer(http.FS(distFS))

	// 注册路由
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 如果是 API 或 WebSocket 路径，跳过
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/ws") ||
			strings.HasPrefix(r.URL.Path, "/webhook/") ||
			strings.HasPrefix(r.URL.Path, "/health") {
			http.NotFound(w, r)
			return
		}

		// 清理路径
		urlPath := path.Clean(r.URL.Path)

		// 如果请求的是根路径或不包含扩展名，返回 index.html
		if urlPath == "/" || urlPath == "" || !strings.Contains(path.Base(urlPath), ".") {
			r.URL.Path = "/index.html"
		}

		// 设置缓存头
		if strings.HasPrefix(urlPath, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}

		logger.Debug("Serving Control UI file",
			zap.String("path", r.URL.Path),
			zap.String("original", urlPath))

		fileServer.ServeHTTP(w, r)
	})

	logger.Info("Control UI registered", zap.String("path", "/"))
	return nil
}
