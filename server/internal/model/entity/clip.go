package entity

import "github.com/gogf/gf/v2/os/gtime"

// Clip 对应数据库 clip 表
type Clip struct {
	Id           int64       `json:"id"           orm:"id"`
	Code         string      `json:"code"         orm:"code"`
	Name         string      `json:"name"         orm:"name"`
	Content      string      `json:"content"      orm:"content"`
	ContentType  string      `json:"contentType"  orm:"content_type"`
	PasswordHash string      `json:"-"            orm:"password_hash"`
	HasPassword  int         `json:"hasPassword"  orm:"has_password"`
	ExpireAt     *gtime.Time `json:"expireAt"     orm:"expire_at"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"`
}

// ClipMessage 对应数据库 clip_message 表
type ClipMessage struct {
	Id          int64       `json:"id"           orm:"id"`
	ClipCode    string      `json:"clipCode"     orm:"clip_code"`
	Content     string      `json:"content"      orm:"content"`
	ContentType string      `json:"contentType"  orm:"content_type"`
	FileName    string      `json:"fileName"     orm:"file_name"`
	FileSize    int64       `json:"fileSize"     orm:"file_size"`
	FileKey     string      `json:"fileKey"      orm:"file_key"`
	CreatedAt   *gtime.Time `json:"createdAt"    orm:"created_at"`
}
