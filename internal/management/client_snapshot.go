package management

import (
	"context"
	"net/http"
)

func (c Client) LoadManagementSnapshot(ctx context.Context) (ManagementSnapshotResponse, error) {
	var out ManagementSnapshotResponse
	if err := c.do(ctx, http.MethodGet, PathSnapshot, nil, &out); err != nil {
		return ManagementSnapshotResponse{}, err
	}
	return out, nil
}
