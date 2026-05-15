package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcmd"
	"github.com/gogf/gf/v2/os/gcron"

	"webclip/server/internal/controller"
	"webclip/server/internal/logic/clip"
)

// Main 是程序主入口命令
var Main = gcmd.Command{
	Name:  "main",
	Usage: "main",
	Brief: "WebClip server",
	Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
		// 确保 data 目录存在
		if err = os.MkdirAll("data", 0o755); err != nil {
			return err
		}

		// 初始化数据库表结构
		if err = clip.Migrate(ctx); err != nil {
			return err
		}

		// 每 10 分钟清理过期数据
		_, _ = gcron.Add(ctx, "0 */10 * * * *", func(ctx context.Context) {
			if err := clip.CleanExpired(ctx); err != nil {
				g.Log().Warningf(ctx, "clean expired err: %v", err)
			}
		})

		s := g.Server()

		clipCtl := controller.NewClip()
		wsCtl := controller.NewWS()

		s.Group("/api", func(group *ghttp.RouterGroup) {
			group.POST("/clip", clipCtl.Create)
			group.GET("/rooms", clipCtl.List)
			group.GET("/clip/{code}/meta", clipCtl.Meta)
			group.POST("/clip/{code}/auth", clipCtl.Auth)
			group.GET("/clip/{code}", clipCtl.Get)
			group.PUT("/clip/{code}", clipCtl.Update)
			group.PATCH("/clip/{code}", clipCtl.Patch)
			group.DELETE("/clip/{code}", clipCtl.Delete)
			group.GET("/clip/{code}/messages", clipCtl.ListMessages)
			group.POST("/clip/{code}/messages", clipCtl.CreateMessage)
			group.DELETE("/clip/{code}/messages/{id}", clipCtl.DeleteMessage)
			group.GET("/ws/{code}", wsCtl.Handle)
		})

		// SPA fallback：非 /api 的请求走静态文件或 index.html
		s.BindHandler("/*any", func(r *ghttp.Request) {
			p := r.URL.Path
			if strings.HasPrefix(p, "/api") {
				r.Response.WriteStatusExit(404, "not found")
				return
			}
			full := filepath.Join("resource/public", p)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				r.Response.ServeFile(full)
				return
			}
			index := "resource/public/index.html"
			if _, err := os.Stat(index); err == nil {
				r.Response.ServeFile(index)
				return
			}
			r.Response.WriteStatusExit(404, "not found")
		})

		s.Run()
		return nil
	},
}
