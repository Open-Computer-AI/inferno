//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type task5SnapshotCache struct {
	testutil.StubConcurrencyCache
	accounts map[int64]service.ConcurrencySnapshot
	users    map[int64]service.ConcurrencySnapshot
	apiKeys  map[int64]service.ConcurrencySnapshot
	err      error
}

func (c *task5SnapshotCache) GetAccountConcurrencySnapshotBatch(context.Context, []int64) (map[int64]service.ConcurrencySnapshot, error) {
	return c.accounts, c.err
}

func (c *task5SnapshotCache) GetUserConcurrencySnapshotBatch(context.Context, []int64) (map[int64]service.ConcurrencySnapshot, error) {
	return c.users, c.err
}

func (c *task5SnapshotCache) GetAPIKeyConcurrencySnapshotBatch(context.Context, []int64) (map[int64]service.ConcurrencySnapshot, error) {
	return c.apiKeys, c.err
}

func TestTask5AdminListRoundTripsClassCountsAndExactAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := newStubAdminService()
	cache := &task5SnapshotCache{
		accounts: map[int64]service.ConcurrencySnapshot{3: {Request: 2, Live: 1, Total: 3}},
		users:    map[int64]service.ConcurrencySnapshot{1: {Request: 4, Live: 2, Total: 6}},
		apiKeys:  map[int64]service.ConcurrencySnapshot{10: {Request: 1, Live: 2, Total: 3}},
	}
	concurrency := service.NewConcurrencyService(cache)

	accountHandler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, concurrency, nil, nil, nil, nil)
	userHandler := NewUserHandler(adminService, concurrency, nil, nil, nil, nil, nil)
	accountRouter := gin.New()
	accountRouter.GET("/api/v1/admin/accounts", accountHandler.List)
	userRouter := gin.New()
	userRouter.GET("/api/v1/admin/users", userHandler.List)
	userRouter.GET("/api/v1/admin/users/:id/api-keys", userHandler.GetUserAPIKeys)

	accountResponse := httptest.NewRecorder()
	accountRouter.ServeHTTP(accountResponse, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, accountResponse.Code)
	var accountsPayload struct {
		Data struct {
			Items []struct {
				CurrentConcurrency        *int `json:"current_concurrency"`
				CurrentLiveConcurrency    *int `json:"current_live_concurrency"`
				CurrentRequestConcurrency *int `json:"current_request_concurrency"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(accountResponse.Body.Bytes(), &accountsPayload))
	require.Len(t, accountsPayload.Data.Items, 1)
	require.Equal(t, 3, *accountsPayload.Data.Items[0].CurrentConcurrency)
	require.Equal(t, 1, *accountsPayload.Data.Items[0].CurrentLiveConcurrency)
	require.Equal(t, 2, *accountsPayload.Data.Items[0].CurrentRequestConcurrency)

	userResponse := httptest.NewRecorder()
	userRouter.ServeHTTP(userResponse, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, userResponse.Code)
	var usersPayload struct {
		Data struct {
			Items []struct {
				CurrentConcurrency        *int `json:"current_concurrency"`
				CurrentLiveConcurrency    *int `json:"current_live_concurrency"`
				CurrentRequestConcurrency *int `json:"current_request_concurrency"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(userResponse.Body.Bytes(), &usersPayload))
	require.Len(t, usersPayload.Data.Items, 1)
	require.Equal(t, 6, *usersPayload.Data.Items[0].CurrentConcurrency)
	require.Equal(t, 2, *usersPayload.Data.Items[0].CurrentLiveConcurrency)
	require.Equal(t, 4, *usersPayload.Data.Items[0].CurrentRequestConcurrency)

	apiKeyResponse := httptest.NewRecorder()
	userRouter.ServeHTTP(apiKeyResponse, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1/api-keys?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, apiKeyResponse.Code)
	var apiKeysPayload struct {
		Data struct {
			Items []struct {
				CurrentConcurrency        *int `json:"current_concurrency"`
				CurrentLiveConcurrency    *int `json:"current_live_concurrency"`
				CurrentRequestConcurrency *int `json:"current_request_concurrency"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(apiKeyResponse.Body.Bytes(), &apiKeysPayload))
	require.Len(t, apiKeysPayload.Data.Items, 1)
	require.Equal(t, 3, *apiKeysPayload.Data.Items[0].CurrentConcurrency)
	require.Equal(t, 2, *apiKeysPayload.Data.Items[0].CurrentLiveConcurrency)
	require.Equal(t, 1, *apiKeysPayload.Data.Items[0].CurrentRequestConcurrency)
}

func TestTask5AdminListShowsUnknownWhenRedisSnapshotFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminService := newStubAdminService()
	concurrency := service.NewConcurrencyService(&task5SnapshotCache{err: errors.New("redis unavailable")})
	accountHandler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, concurrency, nil, nil, nil, nil)
	userHandler := NewUserHandler(adminService, concurrency, nil, nil, nil, nil, nil)
	accountRouter := gin.New()
	accountRouter.GET("/api/v1/admin/accounts", accountHandler.List)
	userRouter := gin.New()
	userRouter.GET("/api/v1/admin/users", userHandler.List)
	userRouter.GET("/api/v1/admin/users/:id/api-keys", userHandler.GetUserAPIKeys)

	for name, router := range map[string]*gin.Engine{"account": accountRouter, "user": userRouter} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			path := "/api/v1/admin/accounts?page=1&page_size=20"
			if name == "user" {
				path = "/api/v1/admin/users?page=1&page_size=20"
			}
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
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
		})
	}

	recorder := httptest.NewRecorder()
	userRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1/api-keys?page=1&page_size=20", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var apiKeysPayload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &apiKeysPayload))
	require.Len(t, apiKeysPayload.Data.Items, 1)
	require.Nil(t, apiKeysPayload.Data.Items[0]["current_concurrency"])
	require.Nil(t, apiKeysPayload.Data.Items[0]["current_live_concurrency"])
	require.Nil(t, apiKeysPayload.Data.Items[0]["current_request_concurrency"])
	require.Equal(t, "unavailable", apiKeysPayload.Data.Items[0]["concurrency_read_error"])
}
