package setting

import (
	"net/http"
	"strings"
	"sync"
	"testing"
)

func resetSystemRequestHeaders() {
	systemRequestHeadersMutex.Lock()
	defer systemRequestHeadersMutex.Unlock()
	SystemRequestHeaders = map[string]string{}
}

func TestCheckSystemRequestHeaders(t *testing.T) {
	cases := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{"empty string is valid (clears config)", "", false},
		{"empty object is valid", "{}", false},
		{"single header", `{"X-Org-Id":"acme"}`, false},
		{"multiple headers", `{"X-Org-Id":"acme","User-Agent":"new-api-system/1.0"}`, false},
		{"empty value allowed", `{"X-Empty":""}`, false},
		{"whitespace around key trimmed", `{"  X-Org-Id  ":"acme"}`, false},
		{"not an object", `["X-Org-Id"]`, true},
		{"plain string", `"X-Org-Id"`, true},
		{"number value rejected", `{"X-Org-Id":123}`, true},
		{"boolean value rejected", `{"X-Org-Id":true}`, true},
		{"nested object value rejected", `{"X-Org-Id":{"a":"b"}}`, true},
		{"null value rejected", `{"X-Org-Id":null}`, true},
		{"blank key rejected", `{"   ":"acme"}`, true},
		{"invalid header name with space", `{"X Org Id":"acme"}`, true},
		{"invalid header name with colon", `{"X:Org":"acme"}`, true},
		{"invalid header name with CJK", `{"组织":"acme"}`, true},
		{"invalid JSON", `{not-json`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckSystemRequestHeaders(tc.jsonStr)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.jsonStr)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.jsonStr, err)
			}
		})
	}
}

func TestCheckSystemRequestHeadersLimits(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < 51; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"X-H-`)
		sb.WriteString(strings.Repeat("a", 1))
		sb.WriteString(string(rune('A' + i%26)))
		sb.WriteString(strings.Repeat("b", i/26))
		sb.WriteString(`":"v"`)
	}
	sb.WriteString("}")
	if err := CheckSystemRequestHeaders(sb.String()); err == nil {
		t.Fatal("expected error for more than 50 headers")
	}

	longValue := `{"X-Long":"` + strings.Repeat("v", 4097) + `"}`
	if err := CheckSystemRequestHeaders(longValue); err == nil {
		t.Fatal("expected error for value longer than 4096 chars")
	}
}

func TestUpdateAndGetSystemRequestHeaders(t *testing.T) {
	resetSystemRequestHeaders()
	t.Cleanup(resetSystemRequestHeaders)

	if got := GetSystemRequestHeaders(); len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}

	err := UpdateSystemRequestHeadersByJSONString(`{"X-Org-Id":"acme","User-Agent":"new-api-system/1.0"}`)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got := GetSystemRequestHeaders()
	if got["X-Org-Id"] != "acme" || got["User-Agent"] != "new-api-system/1.0" {
		t.Fatalf("unexpected headers: %v", got)
	}

	// 副本语义：修改返回值不影响内部状态
	got["X-Org-Id"] = "mutated"
	got["X-Injected"] = "x"
	again := GetSystemRequestHeaders()
	if again["X-Org-Id"] != "acme" {
		t.Fatalf("getter must return a copy, internal state was mutated: %v", again)
	}
	if _, exists := again["X-Injected"]; exists {
		t.Fatalf("getter must return a copy, injected key leaked: %v", again)
	}

	// key 空白字符在 Update 时规范化
	if err := UpdateSystemRequestHeadersByJSONString(`{"  X-Trim  ":"v"}`); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := GetSystemRequestHeaders(); got["X-Trim"] != "v" {
		t.Fatalf("expected trimmed key, got %v", got)
	}

	// 空字符串清空
	if err := UpdateSystemRequestHeadersByJSONString(""); err != nil {
		t.Fatalf("clear failed: %v", err)
	}
	if got := GetSystemRequestHeaders(); len(got) != 0 {
		t.Fatalf("expected cleared map, got %v", got)
	}

	// 非法值拒绝且不污染既有状态
	if err := UpdateSystemRequestHeadersByJSONString(`{"X-Ok":"v"}`); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if err := UpdateSystemRequestHeadersByJSONString(`{"X-Bad":123}`); err == nil {
		t.Fatal("expected error for invalid value")
	}
	if got := GetSystemRequestHeaders(); got["X-Ok"] != "v" {
		t.Fatalf("state polluted by rejected update: %v", got)
	}
}

func TestSystemRequestHeaders2JSONString(t *testing.T) {
	resetSystemRequestHeaders()
	t.Cleanup(resetSystemRequestHeaders)

	if err := UpdateSystemRequestHeadersByJSONString(`{"X-Org-Id":"acme"}`); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := SystemRequestHeaders2JSONString(); got != `{"X-Org-Id":"acme"}` {
		t.Fatalf("unexpected json string: %s", got)
	}
}

func TestApplySystemRequestHeaders(t *testing.T) {
	resetSystemRequestHeaders()
	t.Cleanup(resetSystemRequestHeaders)

	if err := UpdateSystemRequestHeadersByJSONString(`{"X-Org-Id":"acme","User-Agent":"new-api-system/1.0","Authorization":"Bearer global"}`); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	t.Run("fills missing headers", func(t *testing.T) {
		h := http.Header{}
		ApplySystemRequestHeaders(h)
		if h.Get("X-Org-Id") != "acme" || h.Get("User-Agent") != "new-api-system/1.0" {
			t.Fatalf("expected global headers applied, got %v", h)
		}
	})

	t.Run("does not overwrite existing header", func(t *testing.T) {
		h := http.Header{}
		h.Set("Authorization", "Bearer channel-key")
		h.Set("X-Org-Id", "channel-override")
		ApplySystemRequestHeaders(h)
		if h.Get("Authorization") != "Bearer channel-key" {
			t.Fatalf("auth header must not be overwritten, got %q", h.Get("Authorization"))
		}
		if h.Get("X-Org-Id") != "channel-override" {
			t.Fatalf("existing header must not be overwritten, got %q", h.Get("X-Org-Id"))
		}
		if h.Get("User-Agent") != "new-api-system/1.0" {
			t.Fatalf("missing header must be filled, got %q", h.Get("User-Agent"))
		}
	})

	t.Run("case-insensitive absence check", func(t *testing.T) {
		h := http.Header{}
		h.Set("user-agent", "custom")
		ApplySystemRequestHeaders(h)
		if got := h.Values("User-Agent"); len(got) != 1 || got[0] != "custom" {
			t.Fatalf("case variant must count as existing, got %v", h)
		}
	})

	t.Run("empty config is no-op", func(t *testing.T) {
		resetSystemRequestHeaders()
		h := http.Header{}
		h.Set("Authorization", "Bearer k")
		ApplySystemRequestHeaders(h)
		if len(h) != 1 {
			t.Fatalf("expected no changes, got %v", h)
		}
	})
}

func TestSystemRequestHeadersConcurrentAccess(t *testing.T) {
	resetSystemRequestHeaders()
	t.Cleanup(resetSystemRequestHeaders)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = UpdateSystemRequestHeadersByJSONString(`{"X-Org-Id":"acme"}`)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h := http.Header{}
				ApplySystemRequestHeaders(h)
				_ = GetSystemRequestHeaders()
				_ = SystemRequestHeaders2JSONString()
			}
		}()
	}
	wg.Wait()
}
