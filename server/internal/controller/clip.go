package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"

	"webclip/server/internal/logic/clip"
)

// Clip REST 控制器
type Clip struct{}

func NewClip() *Clip { return &Clip{} }

type apiResp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func writeOK(r *ghttp.Request, data interface{}) {
	r.Response.WriteJson(apiResp{Code: 0, Message: "ok", Data: data})
}

func writeErr(r *ghttp.Request, status int, msg string) {
	r.Response.WriteHeader(status)
	r.Response.WriteJson(apiResp{Code: status, Message: msg})
}

func mapErr(r *ghttp.Request, err error) {
	switch {
	case errors.Is(err, clip.ErrNotFound):
		writeErr(r, http.StatusNotFound, err.Error())
	case errors.Is(err, clip.ErrExpired):
		writeErr(r, http.StatusGone, err.Error())
	case errors.Is(err, clip.ErrUnauthorized), errors.Is(err, clip.ErrWrongPassword):
		writeErr(r, http.StatusUnauthorized, err.Error())
	default:
		writeErr(r, http.StatusInternalServerError, err.Error())
	}
}

// bearerToken 解析 Authorization: Bearer <token>
func bearerToken(r *ghttp.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	// 兼容 query
	return r.Get("token").String()
}

// ---- handlers ----

type createReq struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Create POST /api/clip
func (c *Clip) Create(r *ghttp.Request) {
	var req createReq
	_ = r.Parse(&req)

	ctx := r.Context()
	cl, err := clip.Create(ctx, req.Name, req.Password)
	if err != nil {
		mapErr(r, err)
		return
	}
	token, err := clip.IssueToken(ctx, cl.Code)
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{
		"code":        cl.Code,
		"name":        cl.Name,
		"hasPassword": cl.HasPassword == 1,
		"token":       token,
	})
}

// Meta GET /api/clip/{code}/meta
func (c *Clip) Meta(r *ghttp.Request) {
	code := r.Get("code").String()
	cl, err := clip.GetByCode(r.Context(), code)
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{
		"code":        cl.Code,
		"name":        cl.Name,
		"hasPassword": cl.HasPassword == 1,
		"expireAt":    cl.ExpireAt,
		"updatedAt":   cl.UpdatedAt,
	})
}

type authReq struct {
	Password string `json:"password"`
}

// Auth POST /api/clip/{code}/auth
func (c *Clip) Auth(r *ghttp.Request) {
	code := r.Get("code").String()
	var req authReq
	_ = r.Parse(&req)

	ctx := r.Context()
	if err := clip.VerifyPassword(ctx, code, req.Password); err != nil {
		mapErr(r, err)
		return
	}
	token, err := clip.IssueToken(ctx, code)
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{"token": token})
}

// Get GET /api/clip/{code}
func (c *Clip) Get(r *ghttp.Request) {
	code := r.Get("code").String()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	cl, err := clip.GetByCode(ctx, code)
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{
		"code":        cl.Code,
		"content":     cl.Content,
		"contentType": cl.ContentType,
		"updatedAt":   cl.UpdatedAt,
		"expireAt":    cl.ExpireAt,
	})
}

type updateReq struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
}

// Update PUT /api/clip/{code}
func (c *Clip) Update(r *ghttp.Request) {
	code := r.Get("code").String()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req updateReq
	_ = r.Parse(&req)

	cl, err := clip.UpdateContent(ctx, code, req.Content, req.ContentType)
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{
		"code":        cl.Code,
		"content":     cl.Content,
		"contentType": cl.ContentType,
		"updatedAt":   cl.UpdatedAt,
	})
}

// List GET /api/rooms
func (c *Clip) List(r *ghttp.Request) {
	list, err := clip.List(r.Context())
	if err != nil {
		mapErr(r, err)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, cl := range list {
		items = append(items, map[string]interface{}{
			"code":        cl.Code,
			"name":        cl.Name,
			"hasPassword": cl.HasPassword == 1,
			"createdAt":   cl.CreatedAt,
			"updatedAt":   cl.UpdatedAt,
			"expireAt":    cl.ExpireAt,
		})
	}
	writeOK(r, items)
}

type patchReq struct {
	Name     *string `json:"name"`
	Password *string `json:"password"`
}

// Patch PATCH /api/clip/{code} 修改房间名称/密码
func (c *Clip) Patch(r *ghttp.Request) {
	code := r.Get("code").String()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req patchReq
	_ = r.Parse(&req)

	cl, err := clip.UpdateRoom(ctx, code, clip.UpdateRoomOptions{
		Name:     req.Name,
		Password: req.Password,
	})
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{
		"code":        cl.Code,
		"name":        cl.Name,
		"hasPassword": cl.HasPassword == 1,
		"updatedAt":   cl.UpdatedAt,
	})
}

// Delete DELETE /api/clip/{code}
func (c *Clip) Delete(r *ghttp.Request) {
	code := r.Get("code").String()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := clip.Delete(ctx, code); err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{"code": code, "deleted": true})
}

// ---- 消息接口 ----

// ListMessages GET /api/clip/{code}/messages?beforeId=&limit=
func (c *Clip) ListMessages(r *ghttp.Request) {
	code := r.Get("code").String()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	beforeId := r.Get("beforeId").Int64()
	limit := r.Get("limit").Int()
	if limit <= 0 {
		limit = 50
	}
	list, err := clip.ListMessages(ctx, code, beforeId, limit)
	if err != nil {
		mapErr(r, err)
		return
	}
	items := make([]map[string]interface{}, 0, len(list))
	for _, m := range list {
		items = append(items, map[string]interface{}{
			"id":          m.Id,
			"content":     m.Content,
			"contentType": m.ContentType,
			"createdAt":   m.CreatedAt,
		})
	}
	writeOK(r, map[string]interface{}{
		"items":   items,
		"hasMore": len(items) == limit,
	})
}

type sendMessageReq struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"`
}

// CreateMessage POST /api/clip/{code}/messages
func (c *Clip) CreateMessage(r *ghttp.Request) {
	code := r.Get("code").String()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req sendMessageReq
	_ = r.Parse(&req)

	m, err := clip.CreateMessage(ctx, code, req.Content, req.ContentType)
	if err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{
		"id":          m.Id,
		"content":     m.Content,
		"contentType": m.ContentType,
		"createdAt":   m.CreatedAt,
	})
}

// DeleteMessage DELETE /api/clip/{code}/messages/{id}
func (c *Clip) DeleteMessage(r *ghttp.Request) {
	code := r.Get("code").String()
	id := r.Get("id").Int64()
	ctx := r.Context()

	tokenStr := bearerToken(r)
	tokCode, err := clip.VerifyToken(ctx, tokenStr)
	if err != nil || tokCode != code {
		writeErr(r, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := clip.DeleteMessage(ctx, code, id); err != nil {
		mapErr(r, err)
		return
	}
	writeOK(r, map[string]interface{}{"id": id, "deleted": true})
}
