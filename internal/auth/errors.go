package auth

import "errors"

var (
	ErrInvalidAPIKey  = errors.New("API Key不能为空")
	ErrAPIKeyExists   = errors.New("API Key已存在")
	ErrAPIKeyNotFound = errors.New("API Key不存在")
	ErrDeleteAdminKey = errors.New("不能删除管理员密钥")
)
