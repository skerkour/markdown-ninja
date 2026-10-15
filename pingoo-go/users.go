package pingoo

import (
	"context"
	"net/http"

	"markdown.ninja/stdx/uuid"
)

func (client *Client) ListUsers(ctx context.Context, input ListUsersInput) (res PaginatedResult[User], err error) {
	if input.ProjectID == uuid.Nil {
		input.ProjectID = client.projectId
	}

	err = client.request(ctx, requestParams{
		Method:  http.MethodPost,
		Route:   "/users/list",
		Payload: input,
	}, &res)
	return
}

func (client *Client) GetUser(ctx context.Context, input GetUserInput) (user User, err error) {
	err = client.request(ctx, requestParams{
		Method:  http.MethodPost,
		Route:   "/users/get",
		Payload: input,
	}, &user)
	return
}
