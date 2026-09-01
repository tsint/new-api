package setting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// F6：全局系统请求自定义请求头（作用于渠道测试、拉模型、计费查询、任务轮询等系统发起的请求）
// 合并语义为 set-if-absent：渠道级 HeaderOverride 与路径默认认证头优先，全局配置只补缺
var SystemRequestHeaders = map[string]string{}
var systemRequestHeadersMutex sync.RWMutex

const (
	systemRequestHeadersMaxEntries    = 50
	systemRequestHeadersMaxValueLength = 4096
)

func GetSystemRequestHeaders() map[string]string {
	systemRequestHeadersMutex.RLock()
	defer systemRequestHeadersMutex.RUnlock()

	copied := make(map[string]string, len(SystemRequestHeaders))
	for k, v := range SystemRequestHeaders {
		copied[k] = v
	}
	return copied
}

func SystemRequestHeaders2JSONString() string {
	systemRequestHeadersMutex.RLock()
	defer systemRequestHeadersMutex.RUnlock()

	jsonBytes, err := common.Marshal(SystemRequestHeaders)
	if err != nil {
		common.SysLog("error marshalling system request headers: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateSystemRequestHeadersByJSONString(jsonStr string) error {
	parsed, err := parseSystemRequestHeaders(jsonStr)
	if err != nil {
		return err
	}

	systemRequestHeadersMutex.Lock()
	defer systemRequestHeadersMutex.Unlock()
	SystemRequestHeaders = parsed
	return nil
}

func CheckSystemRequestHeaders(jsonStr string) error {
	_, err := parseSystemRequestHeaders(jsonStr)
	return err
}

func parseSystemRequestHeaders(jsonStr string) (map[string]string, error) {
	parsed := make(map[string]string)
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return parsed, nil
	}
	raw := make(map[string]json.RawMessage)
	if err := common.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("invalid system request headers JSON: %w", err)
	}
	normalized := make(map[string]string, len(raw))
	for key, rawValue := range raw {
		if strings.TrimSpace(string(rawValue)) == "null" {
			return nil, fmt.Errorf("header %q has null value", key)
		}
		var value string
		if err := common.Unmarshal(rawValue, &value); err != nil {
			return nil, fmt.Errorf("header %q value must be a string: %w", key, err)
		}
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return nil, fmt.Errorf("header name must not be blank")
		}
		if !isValidSystemRequestHeaderName(trimmedKey) {
			return nil, fmt.Errorf("invalid header name: %q", trimmedKey)
		}
		if len(value) > systemRequestHeadersMaxValueLength {
			return nil, fmt.Errorf("header %q value exceeds %d characters", trimmedKey, systemRequestHeadersMaxValueLength)
		}
		normalized[trimmedKey] = value
	}
	if len(normalized) > systemRequestHeadersMaxEntries {
		return nil, fmt.Errorf("too many headers: %d exceeds limit %d", len(normalized), systemRequestHeadersMaxEntries)
	}
	return normalized, nil
}

// RFC 7230 token 字符集
func isValidSystemRequestHeaderName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		isTokenChar := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '!' || c == '#' || c == '$' || c == '%' || c == '&' || c == '\'' || c == '*' ||
			c == '+' || c == '-' || c == '.' || c == '^' || c == '_' || c == '`' || c == '|' || c == '~'
		if !isTokenChar {
			return false
		}
	}
	return len(name) > 0
}

// ApplySystemRequestHeaders 以 set-if-absent 语义把全局 header 写入 h：
// h 中已存在（大小写不敏感）的 key 不覆盖，仅补充缺失的 key
func ApplySystemRequestHeaders(h http.Header) {
	if h == nil {
		return
	}
	for key, value := range GetSystemRequestHeaders() {
		if _, exists := h[http.CanonicalHeaderKey(key)]; exists {
			continue
		}
		h.Set(key, value)
	}
}
