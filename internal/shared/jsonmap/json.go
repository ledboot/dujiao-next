// Package jsonmap provides the shared JSON object value used at persistence
// and transport boundaries. It is deliberately independent from GORM models
// and business modules so bounded contexts do not need to import a global
// models package merely to exchange arbitrary JSON objects.
package jsonmap

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a JSON object with database/sql serialization support.
type JSON map[string]interface{}

// Value implements driver.Valuer.
func (value JSON) Value() (driver.Value, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

// Scan implements sql.Scanner. SQLite 等驱动按写入类型返回 []byte 或 string：
// 应用自身写入是 BLOB（[]byte），而 SQL 导入/人工修复常写成 TEXT（string），
// 两者都必须能解析，否则 JSON 字段会静默变成 nil。
func (value *JSON) Scan(source interface{}) error {
	if source == nil {
		*value = make(JSON)
		return nil
	}
	var raw []byte
	switch typed := source.(type) {
	case []byte:
		raw = typed
	case string:
		raw = []byte(typed)
	default:
		return fmt.Errorf("jsonmap: unsupported scan source type %T", source)
	}
	return json.Unmarshal(raw, value)
}
