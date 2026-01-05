package lift

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/lists"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestLists_Round12_Coverage(t *testing.T) {
	cfg := round11TestConfig()
	readToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
	readHeaders := map[string]string{"Authorization": "Bearer " + readToken}
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("apiListFromStorage_nil", func(t *testing.T) {
		require.Equal(t, models.List{}, apiListFromStorage(nil))
	})

	t.Run("get_lists_auth_error_returns_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			ListsSvc: &ListsServiceStub{},
		})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists", nil, nil, nil)
		require.NoError(t, err)
		require.Error(t, handler.HandleGetListsLift(ctx))
	})

	t.Run("get_lists_service_error_returns_500", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			ListUserListsFunc: func(_ context.Context, _ *lists.ListUserListsQuery) (*lists.Result, error) {
				return nil, errors.New("boom")
			},
		}

		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists", readHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetListsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("create_list_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/lists", writeHeaders, nil, []byte("{"))
		require.NoError(t, handler.HandleCreateListLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_list_validation_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists", writeHeaders, nil, models.CreateListRequest{Title: ""})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateListLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("create_list_auth_error_returns_error", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			CreateListFunc: func(_ context.Context, _ *lists.CreateListCommand) (*lists.ListResult, error) {
				return &lists.ListResult{List: &storageModels.List{ID: "l1", Title: "t"}}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists", readHeaders, nil, models.CreateListRequest{Title: "t", RepliesPolicy: "list"})
		require.NoError(t, err)
		require.Error(t, handler.HandleCreateListLift(ctx))
	})

	t.Run("create_list_service_error_returns_500", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			CreateListFunc: func(_ context.Context, _ *lists.CreateListCommand) (*lists.ListResult, error) {
				return nil, errors.New("create failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists", writeHeaders, nil, models.CreateListRequest{Title: "t", RepliesPolicy: "list"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleCreateListLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get_list_missing_id_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/", readHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleGetListLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("get_list_auth_error_returns_error", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			GetListFunc: func(_ context.Context, _ *lists.GetListQuery) (*storageModels.List, error) {
				return &storageModels.List{ID: "l1", Title: "t"}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/l1", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.Error(t, handler.HandleGetListLift(ctx))
	})

	t.Run("get_list_not_found_returns_404", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			GetListFunc: func(_ context.Context, _ *lists.GetListQuery) (*storageModels.List, error) {
				return nil, errors.New("not found")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/l1", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleGetListLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("update_list_parse_error_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPut, "/api/v1/lists/l1", writeHeaders, nil, []byte("{"))
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleUpdateListLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update_list_missing_id_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/lists/", writeHeaders, nil, models.UpdateListRequest{Title: "u"})
		require.NoError(t, err)
		require.NoError(t, handler.HandleUpdateListLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("update_list_auth_error_returns_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/lists/l1", nil, nil, models.UpdateListRequest{Title: "u"})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.Error(t, handler.HandleUpdateListLift(ctx))
	})

	t.Run("update_list_service_error_returns_500", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			UpdateListFunc: func(_ context.Context, _ *lists.UpdateListCommand) (*lists.ListResult, error) {
				return nil, errors.New("update failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodPut, "/api/v1/lists/l1", writeHeaders, nil, models.UpdateListRequest{Title: "u"})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleUpdateListLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("delete_list_missing_id_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/", writeHeaders, nil, nil)
		require.NoError(t, err)
		require.NoError(t, handler.HandleDeleteListLift(ctx))
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("delete_list_auth_error_returns_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/l1", nil, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.Error(t, handler.HandleDeleteListLift(ctx))
	})

	t.Run("delete_list_service_error_returns_500", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			DeleteListFunc: func(_ context.Context, _ *lists.DeleteListCommand) error {
				return errors.New("delete failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/l1", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleDeleteListLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("get_list_accounts_not_found_returns_404", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			GetListFunc: func(_ context.Context, _ *lists.GetListQuery) (*storageModels.List, error) {
				return nil, errors.New("not found")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/l1/accounts", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleGetListAccountsLift(ctx))
		require.Equal(t, http.StatusNotFound, ctx.Response.StatusCode)
	})

	t.Run("get_list_accounts_member_without_actor_warns_and_continues", func(t *testing.T) {
		list := &storageModels.List{ID: "l1", Title: "t"}
		listsStub := &ListsServiceStub{
			GetListFunc: func(_ context.Context, _ *lists.GetListQuery) (*storageModels.List, error) {
				return list, nil
			},
			GetListMembersFunc: func(_ context.Context, _ *lists.GetListMembersQuery) (*lists.MembersResult, error) {
				return &lists.MembersResult{
					Members: []*storage.Account{
						{User: &storage.User{Username: "no-actor"}},
						{Actor: &activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/bob"}, PreferredUsername: "bob"}},
					},
				}, nil
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/l1/accounts", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleGetListAccountsLift(ctx))
		require.Equal(t, http.StatusOK, ctx.Response.StatusCode)
	})

	t.Run("get_list_accounts_get_members_error_returns_500", func(t *testing.T) {
		list := &storageModels.List{ID: "l1", Title: "t"}
		listsStub := &ListsServiceStub{
			GetListFunc: func(_ context.Context, _ *lists.GetListQuery) (*storageModels.List, error) {
				return list, nil
			},
			GetListMembersFunc: func(_ context.Context, _ *lists.GetListMembersQuery) (*lists.MembersResult, error) {
				return nil, errors.New("members failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/lists/l1/accounts", readHeaders, nil, nil)
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleGetListAccountsLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("parseAccountIDsRequestWithAuth_missing_list_id_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists//accounts", writeHeaders, nil, models.AddAccountsRequest{AccountIDs: []string{"bob"}})
		require.NoError(t, err)

		_, _, _, parseErr := handler.parseAccountIDsRequestWithAuth(ctx, "add")
		require.NoError(t, parseErr)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("parseAccountIDsRequestWithAuth_empty_account_ids_returns_400", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists/l1/accounts", writeHeaders, nil, models.AddAccountsRequest{AccountIDs: []string{}})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")

		_, _, _, parseErr := handler.parseAccountIDsRequestWithAuth(ctx, "add")
		require.NoError(t, parseErr)
		require.Equal(t, http.StatusBadRequest, ctx.Response.StatusCode)
	})

	t.Run("add_accounts_to_list_loop_error_returns_500", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			AddToListFunc: func(_ context.Context, _ *lists.AddToListCommand) (*lists.MembershipResult, error) {
				return nil, errors.New("add failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists/l1/accounts", writeHeaders, nil, models.AddAccountsRequest{AccountIDs: []string{"bob"}})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleAddAccountsToListLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("add_accounts_to_list_auth_error_returns_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/lists/l1/accounts", readHeaders, nil, models.AddAccountsRequest{AccountIDs: []string{"bob"}})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.Error(t, handler.HandleAddAccountsToListLift(ctx))
	})

	t.Run("remove_accounts_from_list_loop_error_returns_500", func(t *testing.T) {
		listsStub := &ListsServiceStub{
			RemoveFromListFunc: func(_ context.Context, _ *lists.RemoveFromListCommand) (*lists.MembershipResult, error) {
				return nil, errors.New("remove failed")
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: listsStub})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/l1/accounts", writeHeaders, nil, models.RemoveAccountsRequest{AccountIDs: []string{"bob"}})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.NoError(t, handler.HandleRemoveAccountsFromListLift(ctx))
		require.Equal(t, http.StatusInternalServerError, ctx.Response.StatusCode)
	})

	t.Run("remove_accounts_from_list_auth_error_returns_error", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{ListsSvc: &ListsServiceStub{}})

		ctx, err := round10NewLiftContext(http.MethodDelete, "/api/v1/lists/l1/accounts", readHeaders, nil, models.RemoveAccountsRequest{AccountIDs: []string{"bob"}})
		require.NoError(t, err)
		ctx.SetParam("id", "l1")
		require.Error(t, handler.HandleRemoveAccountsFromListLift(ctx))
	})
}
