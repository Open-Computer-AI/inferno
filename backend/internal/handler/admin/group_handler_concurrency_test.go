//go:build unit

package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupHandlerGetGroupAPIKeysIncludesClassConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := newStubAdminService()
	adminService.apiKeys = []service.APIKey{{
		ID:     10,
		UserID: 1,
		Name:   "group-key",
		Status: service.StatusActive,
	}}
	concurrency := service.NewConcurrencyService(&task5SnapshotCache{
		apiKeys: map[int64]service.ConcurrencySnapshot{10: {Request: 2, Live: 3}},
	})
	router := gin.New()
	router.GET("/api/v1/admin/groups/:id/api-keys", NewGroupHandler(adminService, nil, nil, concurrency).GetGroupAPIKeys)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/1/api-keys", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var payload struct {
		Data struct {
			Items []struct {
				CurrentConcurrency        *int `json:"current_concurrency"`
				CurrentLiveConcurrency    *int `json:"current_live_concurrency"`
				CurrentRequestConcurrency *int `json:"current_request_concurrency"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, 5, *payload.Data.Items[0].CurrentConcurrency)
	require.Equal(t, 3, *payload.Data.Items[0].CurrentLiveConcurrency)
	require.Equal(t, 2, *payload.Data.Items[0].CurrentRequestConcurrency)
}

func TestGroupHandlerGetGroupAPIKeysReportsUnknownOnSnapshotFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := newStubAdminService()
	concurrency := service.NewConcurrencyService(&task5SnapshotCache{err: errors.New("redis unavailable")})
	router := gin.New()
	router.GET("/api/v1/admin/groups/:id/api-keys", NewGroupHandler(adminService, nil, nil, concurrency).GetGroupAPIKeys)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups/1/api-keys", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Items, 1)
	require.Nil(t, payload.Data.Items[0]["current_concurrency"])
	require.Nil(t, payload.Data.Items[0]["current_live_concurrency"])
	require.Nil(t, payload.Data.Items[0]["current_request_concurrency"])
	require.Equal(t, "unavailable", payload.Data.Items[0]["concurrency_read_error"])
}
