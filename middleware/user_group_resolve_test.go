package middleware

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func TestResolveUsingGroup(t *testing.T) {
	tests := []struct {
		name       string
		tokenGroup string
		groupList  []string
		want       string
	}{
		{"token group wins", "gb", []string{"ga", "gb"}, "gb"},
		{"multi-group sentinel", "", []string{"ga", "gb"}, ""},
		{"single group concrete", "", []string{"ga"}, "ga"},
		{"auto passes through", "auto", []string{"ga", "gb"}, "auto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveUsingGroup(tt.tokenGroup, tt.groupList); got != tt.want {
				t.Errorf("resolveUsingGroup(%q, %v) = %q, want %q", tt.tokenGroup, tt.groupList, got, tt.want)
			}
		})
	}
}

func TestWriteContextSetsGroupList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	base := &model.UserBase{Group: "ga", Groups: "ga,gb"}
	base.WriteContext(c)
	v, ok := common.GetContextKey(c, constant.ContextKeyUserGroupList)
	if !ok {
		t.Fatal("ContextKeyUserGroupList not set")
	}
	got, _ := v.([]string)
	if !reflect.DeepEqual(got, []string{"ga", "gb"}) {
		t.Errorf("got %v, want [ga gb]", got)
	}
}
