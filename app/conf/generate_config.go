package conf

import (
	"fmt"
	"sync"
)

const (
	keyNotExistsError = "general config key %s not exists"
)

// Config 通用配置
type GeneralConfig struct {
	sync.RWMutex
	m map[string]interface{}
}

func NewGeneralConfig() *GeneralConfig {
	return &GeneralConfig{
		m: make(map[string]interface{}),
	}
}

// GetString 获取指定key的字符串值，如key不存在，返回错误
func (c *GeneralConfig) GetString(key string) (string, error) {
	if v, ok := c.m[key]; ok {
		return v.(string), nil
	}
	return "", fmt.Errorf(keyNotExistsError, key)
}

// GetStringDefault 获取指定key的字符串值，如key不存在，返回默认值
func (c *GeneralConfig) GetStringDefault(key, defaultVal string) string {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	if v, ok := c.m[key]; ok {
		return v.(string)
	}
	return defaultVal
}

func (c *GeneralConfig) GetInt64(key string) (int64, error) {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	if v, ok := c.m[key]; ok {
		return v.(int64), nil
	}
	return 0, fmt.Errorf(keyNotExistsError, key)
}

func (c *GeneralConfig) GetInt64Default(key string, defaultVal int64) int64 {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	if v, ok := c.m[key]; ok {
		return v.(int64)
	}
	return defaultVal
}

func (c *GeneralConfig) GetBool(key string) (bool, error) {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	if v, ok := c.m[key]; ok {
		return v.(bool), nil
	}
	return false, fmt.Errorf(keyNotExistsError, key)
}

func (c *GeneralConfig) GetBoolDefault(key string, defaultVal bool) bool {
	c.RWMutex.RLock()
	defer c.RWMutex.RUnlock()
	if v, ok := c.m[key]; ok {
		return v.(bool)
	}
	return defaultVal
}

func (c *GeneralConfig) Put(key string, value interface{}) {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()
	c.m[key] = value
}

func (c *GeneralConfig) PutAll(m map[string]interface{}) {
	c.RWMutex.Lock()
	defer c.RWMutex.Unlock()
	c.m = m
}
