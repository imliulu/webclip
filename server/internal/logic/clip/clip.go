package clip

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"webclip/server/internal/model/entity"
)

const (
	tableClip    = "clip"
	tableMessage = "clip_message"
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去除易混字符 0/O/1/I
	codeLength   = 6
)

// 业务错误
var (
	ErrNotFound      = gerror.New("clip not found")
	ErrExpired       = gerror.New("clip expired")
	ErrUnauthorized  = gerror.New("unauthorized")
	ErrWrongPassword = gerror.New("wrong password")
)

// Migrate 创建表结构（幂等）
func Migrate(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS clip (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    code           TEXT    NOT NULL UNIQUE,
    name           TEXT    NOT NULL DEFAULT '',
    content        TEXT    NOT NULL DEFAULT '',
    content_type   TEXT    NOT NULL DEFAULT 'text',
    password_hash  TEXT    NOT NULL DEFAULT '',
    has_password   INTEGER NOT NULL DEFAULT 0,
    expire_at      DATETIME,
    created_at     DATETIME,
    updated_at     DATETIME
);
CREATE INDEX IF NOT EXISTS idx_clip_code ON clip(code);
CREATE INDEX IF NOT EXISTS idx_clip_expire ON clip(expire_at);
CREATE TABLE IF NOT EXISTS clip_message (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    clip_code    TEXT    NOT NULL,
    content      TEXT    NOT NULL DEFAULT '',
    content_type TEXT    NOT NULL DEFAULT 'text',
    created_at   DATETIME
);
CREATE INDEX IF NOT EXISTS idx_msg_code_id ON clip_message(clip_code, id DESC);
`
	db := g.DB()
	for _, stmt := range splitStmts(ddl) {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	// 兼容旧版本：若 name 列不存在则补建
	if err := ensureColumn(ctx, "clip", "name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

// ensureColumn 检查表中是否存在某列，不存在则 ALTER TABLE 添加
func ensureColumn(ctx context.Context, table, column, decl string) error {
	rows, err := g.DB().Query(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row["name"].String() == column {
			return nil
		}
	}
	_, err = g.DB().Exec(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

func splitStmts(s string) []string {
	parts := strings.Split(s, ";")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// genCode 生成 6 位短码
func genCode() (string, error) {
	b := make([]byte, codeLength)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := 0; i < codeLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[n.Int64()]
	}
	return string(b), nil
}

// ttl 计算新的过期时间
func ttl(ctx context.Context) *gtime.Time {
	days := g.Cfg().MustGet(ctx, "webclip.ttlDays", 7).Int()
	return gtime.Now().Add(time.Duration(days) * 24 * time.Hour)
}

// Create 创建新房间。password 为空串表示无密码。
func Create(ctx context.Context, name, password string) (*entity.Clip, error) {
	var code string
	var err error
	for i := 0; i < 5; i++ {
		code, err = genCode()
		if err != nil {
			return nil, err
		}
		cnt, cerr := g.DB().Model(tableClip).Ctx(ctx).Where("code", code).Count()
		if cerr != nil {
			return nil, cerr
		}
		if cnt == 0 {
			break
		}
		code = ""
	}
	if code == "" {
		return nil, gerror.New("failed to generate unique code")
	}

	hash := ""
	has := 0
	if password != "" {
		h, berr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if berr != nil {
			return nil, berr
		}
		hash = string(h)
		has = 1
	}
	now := gtime.Now()
	data := g.Map{
		"code":          code,
		"name":          strings.TrimSpace(name),
		"content":       "",
		"content_type":  "text",
		"password_hash": hash,
		"has_password":  has,
		"expire_at":     ttl(ctx),
		"created_at":    now,
		"updated_at":    now,
	}
	if _, err := g.DB().Model(tableClip).Ctx(ctx).Data(data).Insert(); err != nil {
		return nil, err
	}
	return GetByCode(ctx, code)
}

// GetByCode 通过 code 查询
func GetByCode(ctx context.Context, code string) (*entity.Clip, error) {
	one, err := g.DB().Model(tableClip).Ctx(ctx).Where("code", code).One()
	if err != nil {
		return nil, err
	}
	if one.IsEmpty() {
		return nil, ErrNotFound
	}
	var c entity.Clip
	if err = one.Struct(&c); err != nil {
		return nil, err
	}
	if c.ExpireAt != nil && c.ExpireAt.Before(gtime.Now()) {
		return nil, ErrExpired
	}
	return &c, nil
}

// Exists 房间是否存在（不判断过期）
func Exists(ctx context.Context, code string) (bool, error) {
	cnt, err := g.DB().Model(tableClip).Ctx(ctx).Where("code", code).Count()
	if err != nil {
		return false, err
	}
	return cnt > 0, nil
}

// UpdateContent 更新内容并刷新过期时间
func UpdateContent(ctx context.Context, code, content, contentType string) (*entity.Clip, error) {
	if contentType == "" {
		contentType = "text"
	}
	if _, err := GetByCode(ctx, code); err != nil {
		return nil, err
	}
	_, err := g.DB().Model(tableClip).Ctx(ctx).
		Data(g.Map{
			"content":      content,
			"content_type": contentType,
			"expire_at":    ttl(ctx),
			"updated_at":   gtime.Now(),
		}).
		Where("code", code).
		Update()
	if err != nil {
		return nil, err
	}
	return GetByCode(ctx, code)
}

// VerifyPassword 校验房间密码。hasPassword=0 时无论传什么都通过。
func VerifyPassword(ctx context.Context, code, raw string) error {
	one, err := g.DB().Model(tableClip).Ctx(ctx).
		Fields("id, code, password_hash, has_password, expire_at").
		Where("code", code).One()
	if err != nil {
		return err
	}
	if one.IsEmpty() {
		return ErrNotFound
	}
	var c entity.Clip
	if err = one.Struct(&c); err != nil {
		return err
	}
	if c.ExpireAt != nil && c.ExpireAt.Before(gtime.Now()) {
		return ErrExpired
	}
	if c.HasPassword == 0 {
		return nil
	}
	if bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(raw)) != nil {
		return ErrWrongPassword
	}
	return nil
}

// ---- JWT token ----

type tokenClaims struct {
	Code string `json:"code"`
	jwt.RegisteredClaims
}

func jwtSecret(ctx context.Context) []byte {
	s := g.Cfg().MustGet(ctx, "webclip.jwtSecret", "webclip-dev-secret").String()
	return []byte(s)
}

// IssueToken 为房间签发 token
func IssueToken(ctx context.Context, code string) (string, error) {
	hours := g.Cfg().MustGet(ctx, "webclip.tokenExpireHours", 2).Int()
	claims := tokenClaims{
		Code: code,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(hours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(jwtSecret(ctx))
}

// VerifyToken 校验 token，返回 code
func VerifyToken(ctx context.Context, tokenStr string) (string, error) {
	if tokenStr == "" {
		return "", ErrUnauthorized
	}
	claims := &tokenClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(ctx), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrUnauthorized
		}
		return "", ErrUnauthorized
	}
	return claims.Code, nil
}

// CleanExpired 清理过期记录
func CleanExpired(ctx context.Context) error {
	// 先查出过期房间的 code，级联删除它们的消息
	res, err := g.DB().Model(tableClip).Ctx(ctx).
		Fields("code").Where("expire_at < ?", gtime.Now()).All()
	if err != nil {
		return err
	}
	codes := make([]string, 0, len(res))
	for _, row := range res {
		codes = append(codes, row["code"].String())
	}
	if len(codes) > 0 {
		_, _ = g.DB().Model(tableMessage).Ctx(ctx).
			WhereIn("clip_code", codes).Delete()
	}
	_, err = g.DB().Model(tableClip).Ctx(ctx).
		Where("expire_at < ?", gtime.Now()).Delete()
	return err
}

// List 列出所有未过期房间，按更新时间倒序
func List(ctx context.Context) ([]*entity.Clip, error) {
	var list []*entity.Clip
	err := g.DB().Model(tableClip).Ctx(ctx).
		Fields("id, code, name, has_password, expire_at, created_at, updated_at").
		Where("expire_at IS NULL OR expire_at > ?", gtime.Now()).
		Order("updated_at DESC").
		Scan(&list)
	if err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateRoomOptions 更新房间元信息时可选字段
type UpdateRoomOptions struct {
	Name     *string // 非 nil 表示更新名称
	Password *string // 非 nil 表示更新密码（空字符串表示清空密码）
}

// UpdateRoom 修改房间名称 / 密码
func UpdateRoom(ctx context.Context, code string, opts UpdateRoomOptions) (*entity.Clip, error) {
	if _, err := GetByCode(ctx, code); err != nil {
		return nil, err
	}
	data := g.Map{
		"updated_at": gtime.Now(),
	}
	if opts.Name != nil {
		data["name"] = strings.TrimSpace(*opts.Name)
	}
	if opts.Password != nil {
		pw := *opts.Password
		if pw == "" {
			data["password_hash"] = ""
			data["has_password"] = 0
		} else {
			h, berr := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
			if berr != nil {
				return nil, berr
			}
			data["password_hash"] = string(h)
			data["has_password"] = 1
		}
	}
	if _, err := g.DB().Model(tableClip).Ctx(ctx).Data(data).Where("code", code).Update(); err != nil {
		return nil, err
	}
	return GetByCode(ctx, code)
}

// Delete 删除房间
func Delete(ctx context.Context, code string) error {
	if _, err := GetByCode(ctx, code); err != nil {
		return err
	}
	if _, err := g.DB().Model(tableMessage).Ctx(ctx).Where("clip_code", code).Delete(); err != nil {
		return err
	}
	_, err := g.DB().Model(tableClip).Ctx(ctx).Where("code", code).Delete()
	return err
}

// ---- 消息上下文 ----

// ListMessages 按 id 降序获取消息。beforeId<=0 表示最新一页。
func ListMessages(ctx context.Context, code string, beforeId int64, limit int) ([]*entity.ClipMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if _, err := GetByCode(ctx, code); err != nil {
		return nil, err
	}
	m := g.DB().Model(tableMessage).Ctx(ctx).Where("clip_code", code)
	if beforeId > 0 {
		m = m.Where("id < ?", beforeId)
	}
	var list []*entity.ClipMessage
	if err := m.Order("id DESC").Limit(limit).Scan(&list); err != nil {
		return nil, err
	}
	return list, nil
}

// CountMessages 计算房间消息总数
func CountMessages(ctx context.Context, code string) (int, error) {
	return g.DB().Model(tableMessage).Ctx(ctx).Where("clip_code", code).Count()
}

// CreateMessage 创建一条消息，同时刷新房间过期时间与更新时间
func CreateMessage(ctx context.Context, code, content, contentType string) (*entity.ClipMessage, error) {
	if contentType == "" {
		contentType = "text"
	}
	if _, err := GetByCode(ctx, code); err != nil {
		return nil, err
	}
	now := gtime.Now()
	res, err := g.DB().Model(tableMessage).Ctx(ctx).Data(g.Map{
		"clip_code":    code,
		"content":      content,
		"content_type": contentType,
		"created_at":   now,
	}).Insert()
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	// 刷新房间过期时间与 updated_at
	_, _ = g.DB().Model(tableClip).Ctx(ctx).Data(g.Map{
		"expire_at":  ttl(ctx),
		"updated_at": now,
	}).Where("code", code).Update()
	return &entity.ClipMessage{
		Id:          id,
		ClipCode:    code,
		Content:     content,
		ContentType: contentType,
		CreatedAt:   now,
	}, nil
}

// DeleteMessage 删除一条消息
func DeleteMessage(ctx context.Context, code string, id int64) error {
	if _, err := GetByCode(ctx, code); err != nil {
		return err
	}
	cnt, err := g.DB().Model(tableMessage).Ctx(ctx).
		Where("id", id).Where("clip_code", code).Count()
	if err != nil {
		return err
	}
	if cnt == 0 {
		return ErrNotFound
	}
	_, err = g.DB().Model(tableMessage).Ctx(ctx).
		Where("id", id).Where("clip_code", code).Delete()
	return err
}
